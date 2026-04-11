//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/alloydbconn"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoMoney struct {
	CurrencyCode string `bson:"currencyCode"`
	Units        int64  `bson:"units"`
	Nanos        int32  `bson:"nanos"`
}

type mongoProductDocument struct {
	ID          string    `bson:"id"`
	Name        string    `bson:"name"`
	Description string    `bson:"description"`
	Picture     string    `bson:"picture"`
	PriceUSD    mongoMoney `bson:"priceUsd"`
	Categories  []string  `bson:"categories"`
}

func loadCatalog(catalog *ListProductsResponse) error {
	catalogMutex.Lock()
	defer catalogMutex.Unlock()

	if os.Getenv("MONGO_URI") != "" {
		if err := loadCatalogFromMongoDB(catalog); err != nil {
			if strings.EqualFold(os.Getenv("PRODUCTS_FALLBACK_LOCAL"), "true") {
				log.Warnf("failed to load catalog from MongoDB, falling back to products.json: %v", err)
				return loadCatalogFromLocalFile(catalog)
			}
			return err
		}
		return nil
	}

	if os.Getenv("ALLOYDB_CLUSTER_NAME") != "" {
		return loadCatalogFromAlloyDB(catalog)
	}
	if os.Getenv("DB_HOST") != "" {
		return loadCatalogFromPostgres(catalog)
	}

	return loadCatalogFromLocalFile(catalog)
}

func loadCatalogFromMongoDB(catalog *ListProductsResponse) error {
	log.Info("loading catalog from MongoDB...")

	mongoURI := os.Getenv("MONGO_URI")
	databaseName := os.Getenv("MONGO_DATABASE")
	if databaseName == "" {
		databaseName = "catalog_db"
	}
	collectionName := os.Getenv("MONGO_PRODUCTS_COLLECTION")
	if collectionName == "" {
		collectionName = "products"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Warnf("failed to connect to MongoDB: %v", err)
		return err
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	}()

	if err := client.Ping(ctx, nil); err != nil {
		log.Warnf("mongodb ping failed: %v", err)
		return err
	}

	coll := client.Database(databaseName).Collection(collectionName)
	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		log.Warnf("failed to query products collection: %v", err)
		return err
	}
	defer cursor.Close(ctx)

	products := make([]*Product, 0)
	for cursor.Next(ctx) {
		var doc mongoProductDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Warnf("failed to decode mongo product document: %v", err)
			return err
		}

		p := &Product{
			Id:          doc.ID,
			Name:        doc.Name,
			Description: doc.Description,
			Picture:     doc.Picture,
			PriceUsd: &Money{
				CurrencyCode: doc.PriceUSD.CurrencyCode,
				Units:        doc.PriceUSD.Units,
				Nanos:        doc.PriceUSD.Nanos,
			},
			Categories: doc.Categories,
		}
		products = append(products, p)
	}

	if err := cursor.Err(); err != nil {
		log.Warnf("error iterating mongo cursor: %v", err)
		return err
	}

	if len(products) == 0 {
		return fmt.Errorf("mongodb products collection %s.%s is empty", databaseName, collectionName)
	}

	catalog.Products = products
	log.Infof("successfully loaded %d products from MongoDB (%s.%s)", len(products), databaseName, collectionName)
	return nil
}

func loadCatalogFromLocalFile(catalog *ListProductsResponse) error {
	log.Info("loading catalog from local products.json file...")

	catalogJSON, err := os.ReadFile("products.json")
	if err != nil {
		log.Warnf("failed to open product catalog json file: %v", err)
		return err
	}

	if err := json.Unmarshal(catalogJSON, catalog); err != nil {
		log.Warnf("failed to parse the catalog JSON: %v", err)
		return err
	}

	log.Info("successfully parsed product catalog json")
	return nil
}

func getSecretPayload(project, secret, version string) (string, error) {
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Warnf("failed to create SecretManager client: %v", err)
		return "", err
	}
	defer client.Close()

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version),
	}

	// Call the API.
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		log.Warnf("failed to access SecretVersion: %v", err)
		return "", err
	}

	return string(result.Payload.Data), nil
}

func loadCatalogFromAlloyDB(catalog *ListProductsResponse) error {
	log.Info("loading catalog from AlloyDB...")

	projectID := os.Getenv("PROJECT_ID")
	region := os.Getenv("REGION")
	pgClusterName := os.Getenv("ALLOYDB_CLUSTER_NAME")
	pgInstanceName := os.Getenv("ALLOYDB_INSTANCE_NAME")
	pgDatabaseName := os.Getenv("ALLOYDB_DATABASE_NAME")
	pgTableName := os.Getenv("ALLOYDB_TABLE_NAME")
	pgSecretName := os.Getenv("ALLOYDB_SECRET_NAME")

	pgPassword, err := getSecretPayload(projectID, pgSecretName, "latest")
	if err != nil {
		return err
	}

	dialer, err := alloydbconn.NewDialer(context.Background())
	if err != nil {
		log.Warnf("failed to set-up dialer connection: %v", err)
		return err
	}
	cleanup := func() error { return dialer.Close() }
	defer cleanup()

	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s sslmode=disable",
		"postgres", pgPassword, pgDatabaseName,
	)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Warnf("failed to parse DSN config: %v", err)
		return err
	}

	pgInstanceURI := fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", projectID, region, pgClusterName, pgInstanceName)
	config.ConnConfig.DialFunc = func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		return dialer.Dial(ctx, pgInstanceURI)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Warnf("failed to set-up pgx pool: %v", err)
		return err
	}
	defer pool.Close()

	query := "SELECT id, name, description, picture, price_usd_currency_code, price_usd_units, price_usd_nanos, categories FROM " + pgTableName
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		log.Warnf("failed to query database: %v", err)
		return err
	}
	defer rows.Close()

	catalog.Products = catalog.Products[:0]
	for rows.Next() {
		product := &Product{}
		product.PriceUsd = &Money{}

		var categories string
		err = rows.Scan(&product.Id, &product.Name, &product.Description,
			&product.Picture, &product.PriceUsd.CurrencyCode, &product.PriceUsd.Units,
			&product.PriceUsd.Nanos, &categories)
		if err != nil {
			log.Warnf("failed to scan query result row: %v", err)
			return err
		}
		categories = strings.ToLower(categories)
		product.Categories = strings.Split(categories, ",")

		catalog.Products = append(catalog.Products, product)
	}

	log.Info("successfully parsed product catalog from AlloyDB")
	return nil
}

func loadCatalogFromPostgres(catalog *ListProductsResponse) error {
	log.Info("loading catalog from PostgreSQL...")

	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPassword, dbName)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Warnf("failed to parse DSN config: %v", err)
		return err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Warnf("failed to set-up pgx pool: %v", err)
		return err
	}
	defer pool.Close()

	query := "SELECT id, name, description, picture, price_usd_currency_code, price_usd_units, price_usd_nanos, categories FROM products"
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		log.Warnf("failed to query database: %v", err)
		return err
	}
	defer rows.Close()

	catalog.Products = catalog.Products[:0]
	for rows.Next() {
		product := &Product{}
		product.PriceUsd = &Money{}

		var categories string
		err = rows.Scan(&product.Id, &product.Name, &product.Description,
			&product.Picture, &product.PriceUsd.CurrencyCode, &product.PriceUsd.Units,
			&product.PriceUsd.Nanos, &categories)
		if err != nil {
			log.Warnf("failed to scan query result row: %v", err)
			return err
		}
		categories = strings.ToLower(categories)
		product.Categories = strings.Split(categories, ",")

		catalog.Products = append(catalog.Products, product)
	}

	log.Info("successfully parsed product catalog from PostgreSQL")
	return nil
}
