//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	money "github.com/GoogleCloudPlatform/microservices-demo/src/checkoutservice/money"
)

const (
	listenPort  = "5050"
	usdCurrency = "USD"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
	log.Level = logrus.DebugLevel
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

// JSON types matching the proto contract

type CartItem struct {
	ProductId string `json:"productId"`
	Quantity  int32  `json:"quantity"`
}

type Address struct {
	StreetAddress string `json:"streetAddress"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ZipCode       int32  `json:"zipCode"`
}

type CreditCardInfo struct {
	CreditCardNumber          string `json:"creditCardNumber"`
	CreditCardCvv             int32  `json:"creditCardCvv"`
	CreditCardExpirationYear  int32  `json:"creditCardExpirationYear"`
	CreditCardExpirationMonth int32  `json:"creditCardExpirationMonth"`
}

type Product struct {
	Id          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Picture     string       `json:"picture"`
	PriceUsd    *money.Money `json:"priceUsd"`
	Categories  []string     `json:"categories"`
}

type OrderItem struct {
	Item *CartItem    `json:"item"`
	Cost *money.Money `json:"cost"`
}

type OrderResult struct {
	OrderId            string       `json:"orderId"`
	ShippingTrackingId string       `json:"shippingTrackingId"`
	ShippingCost       *money.Money `json:"shippingCost"`
	ShippingAddress    *Address     `json:"shippingAddress"`
	Items              []*OrderItem `json:"items"`
}

type PlaceOrderRequest struct {
	UserId       string          `json:"userId"`
	UserCurrency string          `json:"userCurrency"`
	Address      *Address        `json:"address"`
	Email        string          `json:"email"`
	CreditCard   *CreditCardInfo `json:"creditCard"`
}

type PlaceOrderResponse struct {
	Order *OrderResult `json:"order"`
}

type checkoutService struct {
	productCatalogSvcAddr string
	cartSvcAddr           string
	currencySvcAddr       string
	shippingSvcAddr       string
	emailSvcAddr          string
	paymentSvcAddr        string
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	if os.Getenv("ENABLE_TRACING") == "1" {
		log.Info("Tracing enabled but currently unavailable in REST mode.")
	} else {
		log.Info("Tracing disabled.")
	}

	if os.Getenv("ENABLE_PROFILER") == "1" {
		log.Info("Profiling enabled.")
		go initProfiling("checkoutservice", "1.0.0")
	} else {
		log.Info("Profiling disabled.")
	}

	port := listenPort
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	svc := new(checkoutService)
	mustMapEnv(&svc.shippingSvcAddr, "SHIPPING_SERVICE_ADDR")
	mustMapEnv(&svc.productCatalogSvcAddr, "PRODUCT_CATALOG_SERVICE_ADDR")
	mustMapEnv(&svc.cartSvcAddr, "CART_SERVICE_ADDR")
	mustMapEnv(&svc.currencySvcAddr, "CURRENCY_SERVICE_ADDR")
	mustMapEnv(&svc.emailSvcAddr, "EMAIL_SERVICE_ADDR")
	mustMapEnv(&svc.paymentSvcAddr, "PAYMENT_SERVICE_ADDR")

	log.Infof("service config: %+v", svc)

	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", svc.handlePlaceOrder)
	mux.HandleFunc("/_healthz", handleHealthCheck)

	addr := fmt.Sprintf(":%s", port)
	log.Infof("starting to listen on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func (cs *checkoutService) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Infof("[PlaceOrder] user_id=%q user_currency=%q", req.UserId, req.UserCurrency)

	orderID, err := uuid.NewUUID()
	if err != nil {
		http.Error(w, "failed to generate order uuid", http.StatusInternalServerError)
		return
	}

	prep, err := cs.prepareOrderItemsAndShippingQuoteFromCart(req.UserId, req.UserCurrency, req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total := money.Money{CurrencyCode: req.UserCurrency, Units: 0, Nanos: 0}
	total = money.Must(money.Sum(total, *prep.shippingCostLocalized))
	for _, it := range prep.orderItems {
		multPrice := money.MultiplySlow(*it.Cost, uint32(it.Item.Quantity))
		total = money.Must(money.Sum(total, multPrice))
	}

	txID, err := cs.chargeCard(&total, req.CreditCard)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to charge card: %+v", err), http.StatusInternalServerError)
		return
	}
	log.Infof("payment went through (transaction_id: %s)", txID)

	shippingTrackingID, err := cs.shipOrder(req.Address, prep.cartItems)
	if err != nil {
		http.Error(w, fmt.Sprintf("shipping error: %+v", err), http.StatusInternalServerError)
		return
	}

	_ = cs.emptyUserCart(req.UserId)

	orderResult := &OrderResult{
		OrderId:            orderID.String(),
		ShippingTrackingId: shippingTrackingID,
		ShippingCost:       prep.shippingCostLocalized,
		ShippingAddress:    req.Address,
		Items:              prep.orderItems,
	}

	if err := cs.sendOrderConfirmation(req.Email, orderResult); err != nil {
		log.Warnf("failed to send order confirmation to %q: %+v", req.Email, err)
	} else {
		log.Infof("order confirmation email sent to %q", req.Email)
	}

	resp := PlaceOrderResponse{Order: orderResult}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type orderPrep struct {
	orderItems            []*OrderItem
	cartItems             []CartItem
	shippingCostLocalized *money.Money
}

func (cs *checkoutService) prepareOrderItemsAndShippingQuoteFromCart(userID, userCurrency string, address *Address) (orderPrep, error) {
	var out orderPrep
	cartItems, err := cs.getUserCart(userID)
	if err != nil {
		return out, fmt.Errorf("cart failure: %+v", err)
	}
	orderItems, err := cs.prepOrderItems(cartItems, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to prepare order: %+v", err)
	}
	shippingUSD, err := cs.quoteShipping(address, cartItems)
	if err != nil {
		return out, fmt.Errorf("shipping quote failure: %+v", err)
	}
	shippingPrice, err := cs.convertCurrency(shippingUSD, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to convert shipping cost to currency: %+v", err)
	}

	out.shippingCostLocalized = shippingPrice
	out.cartItems = cartItems
	out.orderItems = orderItems
	return out, nil
}

// REST client helpers

func postJSON(url string, body interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func getJSON(url string, result interface{}) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

type getQuoteResponse struct {
	CostUsd *money.Money `json:"costUsd"`
}

func (cs *checkoutService) quoteShipping(address *Address, items []CartItem) (*money.Money, error) {
	reqBody := map[string]interface{}{
		"address": address,
		"items":   items,
	}
	var resp getQuoteResponse
	if err := postJSON(fmt.Sprintf("http://%s/quote", cs.shippingSvcAddr), reqBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to get shipping quote: %+v", err)
	}
	return resp.CostUsd, nil
}

type cartResponse struct {
	UserId string     `json:"userId"`
	Items  []CartItem `json:"items"`
}

func (cs *checkoutService) getUserCart(userID string) ([]CartItem, error) {
	reqBody := map[string]string{"userId": userID}
	var resp cartResponse
	if err := postJSON(fmt.Sprintf("http://%s/cart/get", cs.cartSvcAddr), reqBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to get user cart during checkout: %+v", err)
	}
	return resp.Items, nil
}

func (cs *checkoutService) emptyUserCart(userID string) error {
	reqBody := map[string]string{"userId": userID}
	return postJSON(fmt.Sprintf("http://%s/cart/empty", cs.cartSvcAddr), reqBody, nil)
}

func (cs *checkoutService) prepOrderItems(items []CartItem, userCurrency string) ([]*OrderItem, error) {
	out := make([]*OrderItem, len(items))
	for i, item := range items {
		var product Product
		if err := getJSON(fmt.Sprintf("http://%s/products/%s", cs.productCatalogSvcAddr, item.ProductId), &product); err != nil {
			return nil, fmt.Errorf("failed to get product #%q", item.ProductId)
		}
		price, err := cs.convertCurrency(product.PriceUsd, userCurrency)
		if err != nil {
			return nil, fmt.Errorf("failed to convert price of %q to %s", item.ProductId, userCurrency)
		}
		out[i] = &OrderItem{
			Item: &item,
			Cost: price,
		}
	}
	return out, nil
}

func (cs *checkoutService) convertCurrency(from *money.Money, toCurrency string) (*money.Money, error) {
	reqBody := map[string]interface{}{
		"from":   from,
		"toCode": toCurrency,
	}
	var result money.Money
	if err := postJSON(fmt.Sprintf("http://%s/convert", cs.currencySvcAddr), reqBody, &result); err != nil {
		return nil, fmt.Errorf("failed to convert currency: %+v", err)
	}
	return &result, nil
}

type chargeResponse struct {
	TransactionId string `json:"transactionId"`
}

func (cs *checkoutService) chargeCard(amount *money.Money, paymentInfo *CreditCardInfo) (string, error) {
	reqBody := map[string]interface{}{
		"amount":     amount,
		"creditCard": paymentInfo,
	}
	var resp chargeResponse
	if err := postJSON(fmt.Sprintf("http://%s/charge", cs.paymentSvcAddr), reqBody, &resp); err != nil {
		return "", fmt.Errorf("could not charge the card: %+v", err)
	}
	return resp.TransactionId, nil
}

func (cs *checkoutService) sendOrderConfirmation(email string, order *OrderResult) error {
	reqBody := map[string]interface{}{
		"email": email,
		"order": order,
	}
	return postJSON(fmt.Sprintf("http://%s/send-confirmation", cs.emailSvcAddr), reqBody, nil)
}

type shipOrderResponse struct {
	TrackingId string `json:"trackingId"`
}

func (cs *checkoutService) shipOrder(address *Address, items []CartItem) (string, error) {
	reqBody := map[string]interface{}{
		"address": address,
		"items":   items,
	}
	var resp shipOrderResponse
	if err := postJSON(fmt.Sprintf("http://%s/shiporder", cs.shippingSvcAddr), reqBody, &resp); err != nil {
		return "", fmt.Errorf("shipment failed: %+v", err)
	}
	return resp.TrackingId, nil
}

func mustMapEnv(target *string, envKey string) {
	v := os.Getenv(envKey)
	if v == "" {
		panic(fmt.Sprintf("environment variable %q not set", envKey))
	}
	*target = v
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
