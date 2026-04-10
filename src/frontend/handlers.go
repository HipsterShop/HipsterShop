//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/microservices-demo/src/frontend/money"
	"github.com/GoogleCloudPlatform/microservices-demo/src/frontend/validator"
)

// JSON types matching the proto contract

type Product struct {
	Id          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Picture     string                 `json:"picture"`
	PriceUsd    *money.Money           `json:"priceUsd"`
	Categories  []string               `json:"categories"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

type CartItem struct {
	ProductId string `json:"productId"`
	Quantity  int32  `json:"quantity"`
}

type Ad struct {
	RedirectUrl string `json:"redirect_url"`
	Text        string `json:"text"`
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

type PlaceOrderResponse struct {
	Order *OrderResult `json:"order"`
}

type platformDetails struct {
	css      string
	provider string
}

var (
	frontendMessage  = strings.TrimSpace(os.Getenv("FRONTEND_MESSAGE"))
	isCymbalBrand    = "true" == strings.ToLower(os.Getenv("CYMBAL_BRANDING"))
	assistantEnabled = "true" == strings.ToLower(os.Getenv("ENABLE_ASSISTANT"))
	templates        = template.Must(template.New("").
				Funcs(template.FuncMap{
			"renderMoney":        renderMoney,
			"renderCurrencyLogo": renderCurrencyLogo,
		}).ParseGlob("templates/*.html"))
	plat platformDetails
)

var validEnvs = []string{"local", "gcp", "azure", "aws", "onprem", "alibaba"}

func (fe *frontendServer) homeHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	log.WithField("currency", currentCurrency(r)).Info("home")
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve currencies"), http.StatusInternalServerError)
		return
	}
	products, err := fe.getProducts(r.Context())
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve products"), http.StatusInternalServerError)
		return
	}
	cart, err := fe.getCart(r.Context(), sessionID(r))
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve cart"), http.StatusInternalServerError)
		return
	}

	type productView struct {
		Item  *Product
		Price *money.Money
	}
	ps := make([]productView, len(products))
	for i, p := range products {
		price, err := fe.convertCurrency(r.Context(), p.PriceUsd, currentCurrency(r))
		if err != nil {
			renderHTTPError(log, r, w, errors.Wrapf(err, "failed to do currency conversion for product %s", p.Id), http.StatusInternalServerError)
			return
		}
		ps[i] = productView{p, price}
	}

	var env = os.Getenv("ENV_PLATFORM")
	if env == "" || stringinSlice(validEnvs, env) == false {
		fmt.Println("env platform is either empty or invalid")
		env = "local"
	}
	addrs, err := net.LookupHost("metadata.google.internal.")
	if err == nil && len(addrs) >= 0 {
		log.Debugf("Detected Google metadata server: %v, setting ENV_PLATFORM to GCP.", addrs)
		env = "gcp"
	}

	log.Debugf("ENV_PLATFORM is: %s", env)
	plat = platformDetails{}
	plat.setPlatformDetails(strings.ToLower(env))

	if err := templates.ExecuteTemplate(w, "home", injectCommonTemplateData(r, map[string]interface{}{
		"show_currency": true,
		"currencies":    currencies,
		"products":      ps,
		"cart_size":     cartSize(cart),
		"banner_color":  os.Getenv("BANNER_COLOR"),
		"ad":            fe.chooseAd(r.Context(), []string{}, log),
	})); err != nil {
		log.Error(err)
	}
}

func (plat *platformDetails) setPlatformDetails(env string) {
	if env == "aws" {
		plat.provider = "AWS"
		plat.css = "aws-platform"
	} else if env == "onprem" {
		plat.provider = "On-Premises"
		plat.css = "onprem-platform"
	} else if env == "azure" {
		plat.provider = "Azure"
		plat.css = "azure-platform"
	} else if env == "gcp" {
		plat.provider = "Google Cloud"
		plat.css = "gcp-platform"
	} else if env == "alibaba" {
		plat.provider = "Alibaba Cloud"
		plat.css = "alibaba-platform"
	} else {
		plat.provider = "local"
		plat.css = "local"
	}
}

func (fe *frontendServer) productHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	id := mux.Vars(r)["id"]
	if id == "" {
		renderHTTPError(log, r, w, errors.New("product id not specified"), http.StatusBadRequest)
		return
	}
	log.WithField("id", id).WithField("currency", currentCurrency(r)).
		Debug("serving product page")

	p, err := fe.getProduct(r.Context(), id)
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve product"), http.StatusInternalServerError)
		return
	}
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve currencies"), http.StatusInternalServerError)
		return
	}

	cart, err := fe.getCart(r.Context(), sessionID(r))
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve cart"), http.StatusInternalServerError)
		return
	}

	price, err := fe.convertCurrency(r.Context(), p.PriceUsd, currentCurrency(r))
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "failed to convert currency"), http.StatusInternalServerError)
		return
	}

	recommendations, err := fe.getRecommendations(r.Context(), sessionID(r), []string{id})
	if err != nil {
		log.WithField("error", err).Warn("failed to get product recommendations")
	}

	product := struct {
		Item  *Product
		Price *money.Money
	}{p, price}

	var packagingInfo *PackagingInfo = nil
	if isPackagingServiceConfigured() {
		packagingInfo, err = httpGetPackagingInfo(id)
		if err != nil {
			fmt.Println("Failed to obtain product's packaging info:", err)
		}
	}

	if err := templates.ExecuteTemplate(w, "product", injectCommonTemplateData(r, map[string]interface{}{
		"ad":              fe.chooseAd(r.Context(), p.Categories, log),
		"show_currency":   true,
		"currencies":      currencies,
		"product":         product,
		"recommendations": recommendations,
		"cart_size":       cartSize(cart),
		"packagingInfo":   packagingInfo,
	})); err != nil {
		log.Println(err)
	}
}

func (fe *frontendServer) addToCartHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	userID := authenticatedUserID(r)
	if userID == "" {
		renderHTTPError(log, r, w, errors.New("authentication required"), http.StatusUnauthorized)
		return
	}
	quantity, _ := strconv.ParseUint(r.FormValue("quantity"), 10, 32)
	productID := r.FormValue("product_id")
	payload := validator.AddToCartPayload{
		Quantity:  quantity,
		ProductID: productID,
	}
	if err := payload.Validate(); err != nil {
		renderHTTPError(log, r, w, validator.ValidationErrorResponse(err), http.StatusUnprocessableEntity)
		return
	}
	log.WithField("product", payload.ProductID).WithField("quantity", payload.Quantity).Debug("adding to cart")

	p, err := fe.getProduct(r.Context(), payload.ProductID)
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve product"), http.StatusInternalServerError)
		return
	}

	if err := fe.insertCart(r.Context(), userID, p.Id, int32(payload.Quantity)); err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "failed to add to cart"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("location", baseUrl + "/cart")
	w.WriteHeader(http.StatusFound)
}

func (fe *frontendServer) emptyCartHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	userID := authenticatedUserID(r)
	if userID == "" {
		renderHTTPError(log, r, w, errors.New("authentication required"), http.StatusUnauthorized)
		return
	}
	log.Debug("emptying cart")

	if err := fe.emptyCart(r.Context(), userID); err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "failed to empty cart"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("location", baseUrl + "/")
	w.WriteHeader(http.StatusFound)
}

func (fe *frontendServer) viewCartHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	userID := authenticatedUserID(r)
	if userID == "" {
		renderHTTPError(log, r, w, errors.New("authentication required"), http.StatusUnauthorized)
		return
	}
	log.Debug("view user cart")
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve currencies"), http.StatusInternalServerError)
		return
	}
	cart, err := fe.getCart(r.Context(), userID)
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve cart"), http.StatusInternalServerError)
		return
	}

	recommendations, err := fe.getRecommendations(r.Context(), userID, cartIDs(cart))
	if err != nil {
		log.WithField("error", err).Warn("failed to get product recommendations")
	}

	shippingCost, err := fe.getShippingQuote(r.Context(), cart, currentCurrency(r))
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "failed to get shipping quote"), http.StatusInternalServerError)
		return
	}

	type cartItemView struct {
		Item     *Product
		Quantity int32
		Price    *money.Money
	}
	items := make([]cartItemView, len(cart))
	totalPrice := money.Money{CurrencyCode: currentCurrency(r)}
	for i, item := range cart {
		p, err := fe.getProduct(r.Context(), item.ProductId)
		if err != nil {
			renderHTTPError(log, r, w, errors.Wrapf(err, "could not retrieve product #%s", item.ProductId), http.StatusInternalServerError)
			return
		}
		price, err := fe.convertCurrency(r.Context(), p.PriceUsd, currentCurrency(r))
		if err != nil {
			renderHTTPError(log, r, w, errors.Wrapf(err, "could not convert currency for product #%s", item.ProductId), http.StatusInternalServerError)
			return
		}

		multPrice := money.MultiplySlow(*price, uint32(item.Quantity))
		items[i] = cartItemView{
			Item:     p,
			Quantity: item.Quantity,
			Price:    &multPrice}
		totalPrice = money.Must(money.Sum(totalPrice, multPrice))
	}
	totalPrice = money.Must(money.Sum(totalPrice, *shippingCost))
	year := time.Now().Year()

	if err := templates.ExecuteTemplate(w, "cart", injectCommonTemplateData(r, map[string]interface{}{
		"currencies":       currencies,
		"recommendations":  recommendations,
		"cart_size":        cartSize(cart),
		"shipping_cost":    shippingCost,
		"show_currency":    true,
		"total_cost":       totalPrice,
		"items":            items,
		"expiration_years": []int{year, year + 1, year + 2, year + 3, year + 4},
	})); err != nil {
		log.Println(err)
	}
}

func (fe *frontendServer) placeOrderHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	userID := authenticatedUserID(r)
	if userID == "" {
		renderHTTPError(log, r, w, errors.New("authentication required"), http.StatusUnauthorized)
		return
	}
	log.Debug("placing order")

	var (
		email         = r.FormValue("email")
		streetAddress = r.FormValue("street_address")
		zipCode, _    = strconv.ParseInt(r.FormValue("zip_code"), 10, 32)
		city          = r.FormValue("city")
		state         = r.FormValue("state")
		country       = r.FormValue("country")
		ccNumber      = r.FormValue("credit_card_number")
		ccMonth, _    = strconv.ParseInt(r.FormValue("credit_card_expiration_month"), 10, 32)
		ccYear, _     = strconv.ParseInt(r.FormValue("credit_card_expiration_year"), 10, 32)
		ccCVV, _      = strconv.ParseInt(r.FormValue("credit_card_cvv"), 10, 32)
	)

	payload := validator.PlaceOrderPayload{
		Email:         email,
		StreetAddress: streetAddress,
		ZipCode:       zipCode,
		City:          city,
		State:         state,
		Country:       country,
		CcNumber:      ccNumber,
		CcMonth:       ccMonth,
		CcYear:        ccYear,
		CcCVV:         ccCVV,
	}
	if err := payload.Validate(); err != nil {
		renderHTTPError(log, r, w, validator.ValidationErrorResponse(err), http.StatusUnprocessableEntity)
		return
	}

	// Call checkout service via REST
	checkoutReq := map[string]interface{}{
		"email": payload.Email,
		"creditCard": map[string]interface{}{
			"creditCardNumber":          payload.CcNumber,
			"creditCardExpirationMonth": int32(payload.CcMonth),
			"creditCardExpirationYear":  int32(payload.CcYear),
			"creditCardCvv":             int32(payload.CcCVV),
		},
		"userId":       userID,
		"userCurrency": currentCurrency(r),
		"address": map[string]interface{}{
			"streetAddress": payload.StreetAddress,
			"city":          payload.City,
			"state":         payload.State,
			"zipCode":       int32(payload.ZipCode),
			"country":       payload.Country,
		},
	}

	var order PlaceOrderResponse
	if err := postJSON(fmt.Sprintf("http://%s/api/checkout", fe.gatewaySvcAddr), checkoutReq, &order); err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "failed to complete the order"), http.StatusInternalServerError)
		return
	}
	log.WithField("order", order.Order.OrderId).Info("order placed")

	recommendations, _ := fe.getRecommendations(r.Context(), userID, nil)

	totalPaid := *order.Order.ShippingCost
	for _, v := range order.Order.Items {
		multPrice := money.MultiplySlow(*v.Cost, uint32(v.Item.Quantity))
		totalPaid = money.Must(money.Sum(totalPaid, multPrice))
	}

	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		renderHTTPError(log, r, w, errors.Wrap(err, "could not retrieve currencies"), http.StatusInternalServerError)
		return
	}

	if err := templates.ExecuteTemplate(w, "order", injectCommonTemplateData(r, map[string]interface{}{
		"show_currency":   false,
		"currencies":      currencies,
		"order":           order.Order,
		"total_paid":      &totalPaid,
		"recommendations": recommendations,
	})); err != nil {
		log.Println(err)
	}
}

// razorpayOrderConfirmHandler renders the order-confirmation page after a
// successful Razorpay payment. The browser JS redirects here with orderId and
// txnId as query parameters after the verify handler returns success.
func (fe *frontendServer) razorpayOrderConfirmHandler(w http.ResponseWriter, r *http.Request) {
	orderId := r.URL.Query().Get("orderId")
	txnId   := r.URL.Query().Get("txnId")

	if err := templates.ExecuteTemplate(w, "razorpay-confirm", injectCommonTemplateData(r, map[string]interface{}{
		"order_id": orderId,
		"txn_id":   txnId,
	})); err != nil {
		log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
		log.Println(err)
	}
}

// razorpayVerifyRequest is the JSON body sent by the browser after Razorpay popup success.
type razorpayVerifyRequest struct {
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpaySignature string `json:"razorpay_signature"`
	Email             string `json:"email"`
	StreetAddress     string `json:"street_address"`
	ZipCode           string `json:"zip_code"`
	City              string `json:"city"`
	State             string `json:"state"`
	Country           string `json:"country"`
	Currency          string `json:"currency"`
}

// razorpayVerifyHandler is called by the browser JS after a successful Razorpay popup payment.
// It:
//  1. Verifies the HMAC signature with the paymentservice
//  2. Places the order through the checkoutservice
//  3. Returns JSON { orderId, transactionId } for the browser to redirect
func (fe *frontendServer) razorpayVerifyHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	userID := authenticatedUserID(r)
	if userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"authentication required"}`)
		return
	}

	var req razorpayVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid request body"}`)
		return
	}

	log.WithFields(logrus.Fields{
		"razorpay_order_id":   req.RazorpayOrderID,
		"razorpay_payment_id": req.RazorpayPaymentID,
	}).Info("razorpay verify: verifying payment signature")

	// ── 1. Verify Razorpay payment signature via paymentservice ──────────────
	verifyPayload := map[string]string{
		"razorpay_payment_id": req.RazorpayPaymentID,
		"razorpay_order_id":   req.RazorpayOrderID,
		"razorpay_signature":  req.RazorpaySignature,
	}
	var verifyResult struct {
		TransactionID string `json:"transactionId"`
	}
	if err := postJSON(
		fmt.Sprintf("http://%s/api/payment/verify", fe.gatewaySvcAddr),
		verifyPayload,
		&verifyResult,
	); err != nil {
		log.WithField("error", err).Error("razorpay verify: signature verification failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Payment verification failed: " + err.Error()})
		return
	}

	log.WithField("transactionId", verifyResult.TransactionID).Info("razorpay verify: payment verified, placing order")

	// ── 2. Place order via checkoutservice ───────────────────────────────────
	currency := req.Currency
	if currency == "" {
		currency = currentCurrency(r)
	}
	zipCode, _ := strconv.ParseInt(req.ZipCode, 10, 32)

	checkoutReq := map[string]interface{}{
		"email": req.Email,
		// Payment was already collected by Razorpay; pass dummy card data
		// so the checkoutservice schema is satisfied.
		"creditCard": map[string]interface{}{
			"creditCardNumber":          "4111111111111111",
			"creditCardExpirationMonth": int32(1),
			"creditCardExpirationYear":  int32(2035),
			"creditCardCvv":             int32(000),
		},
		"userId":       userID,
		"userCurrency": currency,
		"address": map[string]interface{}{
			"streetAddress": req.StreetAddress,
			"city":          req.City,
			"state":         req.State,
			"zipCode":       int32(zipCode),
			"country":       req.Country,
		},
	}

	var order PlaceOrderResponse
	if err := postJSON(
		fmt.Sprintf("http://%s/api/checkout", fe.gatewaySvcAddr),
		checkoutReq,
		&order,
	); err != nil {
		log.WithField("error", err).Error("razorpay verify: order placement failed after payment verification")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Order could not be placed after payment"})
		return
	}

	log.WithFields(logrus.Fields{
		"orderId":       order.Order.OrderId,
		"transactionId": verifyResult.TransactionID,
	}).Info("razorpay verify: order placed successfully")

	// ── 3. Return order info to browser JS ───────────────────────────────────
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"orderId":       order.Order.OrderId,
		"transactionId": verifyResult.TransactionID,
	})
}

func (fe *frontendServer) loginHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	if isAuthenticated(r) {
		w.Header().Set("Location", baseUrl+"/")
		w.WriteHeader(http.StatusFound)
		return
	}
	if err := templates.ExecuteTemplate(w, "login.html", injectCommonTemplateData(r, map[string]interface{}{})); err != nil {
		log.Println(err)
	}
}

func (fe *frontendServer) loginPostHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		if err := templates.ExecuteTemplate(w, "login.html", injectCommonTemplateData(r, map[string]interface{}{
			"error": "Email and password are required",
		})); err != nil {
			log.Println(err)
		}
		return
	}

	authCookies, err := fe.authLogin(r.Context(), email, password)
	if err != nil {
		if err := templates.ExecuteTemplate(w, "login.html", injectCommonTemplateData(r, map[string]interface{}{
			"error": err.Error(),
		})); err != nil {
			log.Println(err)
		}
		return
	}
	forwardAuthTokenCookie(w, authCookies)
	w.Header().Set("Location", baseUrl+"/cart")
	w.WriteHeader(http.StatusFound)
}

func (fe *frontendServer) signupHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	if isAuthenticated(r) {
		w.Header().Set("Location", baseUrl+"/")
		w.WriteHeader(http.StatusFound)
		return
	}
	if err := templates.ExecuteTemplate(w, "signup.html", injectCommonTemplateData(r, map[string]interface{}{})); err != nil {
		log.Println(err)
	}
}

func (fe *frontendServer) signupPostHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		if err := templates.ExecuteTemplate(w, "signup.html", injectCommonTemplateData(r, map[string]interface{}{
			"error": "Email and password are required",
		})); err != nil {
			log.Println(err)
		}
		return
	}

	authCookies, err := fe.authSignup(r.Context(), name, email, password)
	if err != nil {
		if err := templates.ExecuteTemplate(w, "signup.html", injectCommonTemplateData(r, map[string]interface{}{
			"error": err.Error(),
		})); err != nil {
			log.Println(err)
		}
		return
	}
	forwardAuthTokenCookie(w, authCookies)
	w.Header().Set("Location", baseUrl+"/cart")
	w.WriteHeader(http.StatusFound)
}

func forwardAuthTokenCookie(w http.ResponseWriter, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		if cookie.Name == "auth_token" {
			http.SetCookie(w, cookie)
			return
		}
	}
}



func (fe *frontendServer) logoutHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	log.Debug("logging out")
	for _, c := range r.Cookies() {
		c.Expires = time.Now().Add(-time.Hour * 24 * 365)
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
	w.Header().Set("Location", baseUrl + "/")
	w.WriteHeader(http.StatusFound)
}

func (fe *frontendServer) getProductByID(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["ids"]
	if id == "" {
		return
	}

	p, err := fe.getProduct(r.Context(), id)
	if err != nil {
		return
	}

	jsonData, err := json.Marshal(p)
	if err != nil {
		fmt.Println(err)
		return
	}

	w.Write(jsonData)
	w.WriteHeader(http.StatusOK)
}



func (fe *frontendServer) setCurrencyHandler(w http.ResponseWriter, r *http.Request) {
	log := r.Context().Value(ctxKeyLog{}).(logrus.FieldLogger)
	cur := r.FormValue("currency_code")
	payload := validator.SetCurrencyPayload{Currency: cur}
	if err := payload.Validate(); err != nil {
		renderHTTPError(log, r, w, validator.ValidationErrorResponse(err), http.StatusUnprocessableEntity)
		return
	}
	log.WithField("curr.new", payload.Currency).WithField("curr.old", currentCurrency(r)).
		Debug("setting currency")

	if payload.Currency != "" {
		http.SetCookie(w, &http.Cookie{
			Name:   cookieCurrency,
			Value:  payload.Currency,
			MaxAge: cookieMaxAge,
		})
	}
	referer := r.Header.Get("referer")
	if referer == "" {
		referer = baseUrl + "/"
	}
	w.Header().Set("Location", referer)
	w.WriteHeader(http.StatusFound)
}

// chooseAd queries for advertisements available and randomly chooses one, if
// available. It ignores the error retrieving the ad since it is not critical.
func (fe *frontendServer) chooseAd(ctx context.Context, ctxKeys []string, log logrus.FieldLogger) *Ad {
	ads, err := fe.getAd(ctx, ctxKeys)
	if err != nil {
		log.WithField("error", err).Warn("failed to retrieve ads")
		return nil
	}
	return ads[rand.Intn(len(ads))]
}

func renderHTTPError(log logrus.FieldLogger, r *http.Request, w http.ResponseWriter, err error, code int) {
	log.WithField("error", err).Error("request error")
	errMsg := fmt.Sprintf("%+v", err)

	w.WriteHeader(code)

	if templateErr := templates.ExecuteTemplate(w, "error", injectCommonTemplateData(r, map[string]interface{}{
		"error":       errMsg,
		"status_code": code,
		"status":      http.StatusText(code),
	})); templateErr != nil {
		log.Println(templateErr)
	}
}

func injectCommonTemplateData(r *http.Request, payload map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"session_id":        sessionID(r),
		"request_id":        r.Context().Value(ctxKeyRequestID{}),
		"is_authenticated":  isAuthenticated(r),
		"user_email":        authenticatedUserEmail(r),
		"user_currency":     currentCurrency(r),
		"platform_css":      plat.css,
		"platform_name":     plat.provider,
		"is_cymbal_brand":   isCymbalBrand,
		"assistant_enabled": assistantEnabled,
		"deploymentDetails": deploymentDetailsMap,
		"frontendMessage":   frontendMessage,
		"currentYear":       time.Now().Year(),
		"baseUrl":           baseUrl,
	}

	for k, v := range payload {
		data[k] = v
	}

	return data
}

func currentCurrency(r *http.Request) string {
	c, _ := r.Cookie(cookieCurrency)
	if c != nil {
		return c.Value
	}
	return defaultCurrency
}

func sessionID(r *http.Request) string {
	v := r.Context().Value(ctxKeySessionID{})
	if v != nil {
		return v.(string)
	}
	return ""
}

func isAuthenticated(r *http.Request) bool {
	v := r.Context().Value(ctxKeyAuthenticated{})
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func authenticatedUserID(r *http.Request) string {
	v := r.Context().Value(ctxKeyUserID{})
	if v == nil {
		return ""
	}
	userID, ok := v.(string)
	if !ok {
		return ""
	}
	return userID
}

func authenticatedUserEmail(r *http.Request) string {
	v := r.Context().Value(ctxKeyUserEmail{})
	if v == nil {
		return ""
	}
	email, ok := v.(string)
	if !ok {
		return ""
	}
	return email
}

func cartIDs(c []*CartItem) []string {
	out := make([]string, len(c))
	for i, v := range c {
		out[i] = v.ProductId
	}
	return out
}

// get total # of items in cart
func cartSize(c []*CartItem) int {
	cartSize := 0
	for _, item := range c {
		cartSize += int(item.Quantity)
	}
	return cartSize
}

func renderMoney(m money.Money) string {
	currencyLogo := renderCurrencyLogo(m.CurrencyCode)
	return fmt.Sprintf("%s%d.%02d", currencyLogo, m.Units, m.Nanos/10000000)
}

func renderCurrencyLogo(currencyCode string) string {
	logos := map[string]string{
		"USD": "$",
		"CAD": "$",
		"JPY": "¥",
		"EUR": "€",
		"TRY": "₺",
		"GBP": "£",
	}

	logo := "$" //default
	if val, ok := logos[currencyCode]; ok {
		logo = val
	}
	return logo
}

func stringinSlice(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
