//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/sirupsen/logrus"
)

var (
	catalogMutex *sync.Mutex
	log          *logrus.Logger
	extraLatency time.Duration

	port = "3550"

	reloadCatalog bool
)

// JSON data types matching the proto contract

type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

type Product struct {
	Id          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Picture     string                 `json:"picture"`
	PriceUsd    *Money                 `json:"priceUsd"`
	Categories  []string               `json:"categories"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

type ListProductsResponse struct {
	Products []*Product `json:"products"`
}

type SearchProductsResponse struct {
	Results []*Product `json:"results"`
}

type productCatalog struct {
	catalog ListProductsResponse
}

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
	catalogMutex = &sync.Mutex{}
}

var svc *productCatalog

func main() {
	if os.Getenv("ENABLE_TRACING") == "1" {
		log.Info("Tracing enabled but currently unavailable in REST mode.")
	} else {
		log.Info("Tracing disabled.")
	}

	if os.Getenv("DISABLE_PROFILER") == "" {
		log.Info("Profiling enabled.")
		go initProfiling("productcatalogservice", "1.0.0")
	} else {
		log.Info("Profiling disabled.")
	}

	// set injected latency
	if s := os.Getenv("EXTRA_LATENCY"); s != "" {
		v, err := time.ParseDuration(s)
		if err != nil {
			log.Fatalf("failed to parse EXTRA_LATENCY (%s) as time.Duration: %+v", v, err)
		}
		extraLatency = v
		log.Infof("extra latency enabled (duration: %v)", extraLatency)
	} else {
		extraLatency = time.Duration(0)
	}

	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	svc = &productCatalog{}
	if err := loadCatalog(&svc.catalog); err != nil {
		log.Fatalf("could not parse product catalog: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/products", handleProducts)
	mux.HandleFunc("/products/search", handleSearchProducts)
	mux.HandleFunc("/products/", handleGetProduct)
	mux.HandleFunc("/_healthz", handleHealthCheck)

	addr := fmt.Sprintf(":%s", port)
	log.Infof("starting REST server at %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	time.Sleep(extraLatency)

	resp := ListProductsResponse{Products: svc.parseCatalog()}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleGetProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	time.Sleep(extraLatency)

	// Extract product ID from URL path: /products/{id}
	id := strings.TrimPrefix(r.URL.Path, "/products/")
	if id == "" {
		http.Error(w, "product id not specified", http.StatusBadRequest)
		return
	}

	var found *Product
	for _, p := range svc.parseCatalog() {
		if p.Id == id {
			found = p
			break
		}
	}

	if found == nil {
		http.Error(w, fmt.Sprintf("no product with ID %s", id), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(found)
}

func handleSearchProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	time.Sleep(extraLatency)

	query := r.URL.Query().Get("q")
	var results []*Product
	for _, product := range svc.parseCatalog() {
		if strings.Contains(strings.ToLower(product.Name), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(product.Description), strings.ToLower(query)) {
			results = append(results, product)
		}
	}

	resp := SearchProductsResponse{Results: results}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func (p *productCatalog) parseCatalog() []*Product {
	if reloadCatalog || len(p.catalog.Products) == 0 {
		err := loadCatalog(&p.catalog)
		if err != nil {
			return []*Product{}
		}
	}
	return p.catalog.Products
}

func initProfiling(service, version string) {
	for i := 1; i <= 3; i++ {
		if err := profiler.Start(profiler.Config{
			Service:        service,
			ServiceVersion: version,
		}); err != nil {
			log.Warnf("failed to start profiler: %+v", err)
		} else {
			log.Info("started Stackdriver profiler")
			return
		}
		d := time.Second * 10 * time.Duration(i)
		log.Infof("sleeping %v to retry initializing Stackdriver profiler", d)
		time.Sleep(d)
	}
	log.Warn("could not initialize Stackdriver profiler after retrying, giving up")
}
