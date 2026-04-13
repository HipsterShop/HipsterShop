//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/microservices-demo/src/frontend/money"
	"github.com/pkg/errors"
)

const (
	avoidNoopCurrencyConversionRPC = false
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("request failed with status %d", e.Status)
	}
	return e.Message
}

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

func postJSONWithCookies(ctx context.Context, url string, body interface{}) ([]*http.Cookie, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Cookies(), nil
}

func decodeAPIError(resp *http.Response) error {
	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
		if msg, ok := payload["error"].(string); ok && msg != "" {
			return &apiError{Status: resp.StatusCode, Message: msg}
		}
	}
	return &apiError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
}

func (fe *frontendServer) authLogin(ctx context.Context, email, password string) ([]*http.Cookie, error) {
	reqBody := map[string]string{"email": email, "password": password}
	cookies, err := postJSONWithCookies(ctx, fmt.Sprintf("http://%s/api/auth/login", fe.gatewaySvcAddr), reqBody)
	if err != nil {
		if apiErr, ok := err.(*apiError); ok {
			if apiErr.Status == http.StatusUnauthorized {
				return nil, fmt.Errorf("Invalid email or password")
			}
			return nil, fmt.Errorf("%s", apiErr.Message)
		}
		return nil, err
	}
	return cookies, nil
}

func (fe *frontendServer) authSignup(ctx context.Context, name, email, password string) ([]*http.Cookie, error) {
	reqBody := map[string]string{"name": name, "email": email, "password": password}
	cookies, err := postJSONWithCookies(ctx, fmt.Sprintf("http://%s/api/auth/signup", fe.gatewaySvcAddr), reqBody)
	if err != nil {
		if apiErr, ok := err.(*apiError); ok {
			if apiErr.Status == http.StatusConflict {
				return nil, fmt.Errorf("Email already registered")
			}
			return nil, fmt.Errorf("%s", apiErr.Message)
		}
		return nil, err
	}
	return cookies, nil
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

type getSupportedCurrenciesResponse struct {
	CurrencyCodes []string `json:"currencyCodes"`
}

func (fe *frontendServer) getCurrencies(ctx context.Context) ([]string, error) {
	var resp getSupportedCurrenciesResponse
	if err := getJSON(fmt.Sprintf("http://%s/api/currency/currencies", fe.gatewaySvcAddr), &resp); err != nil {
		return nil, err
	}
	var out []string
	for _, c := range resp.CurrencyCodes {
		if _, ok := whitelistedCurrencies[c]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

type listProductsResponse struct {
	Products []*Product `json:"products"`
}

func (fe *frontendServer) getProducts(ctx context.Context) ([]*Product, error) {
	var resp listProductsResponse
	if err := getJSON(fmt.Sprintf("http://%s/api/products", fe.gatewaySvcAddr), &resp); err != nil {
		return nil, err
	}
	return resp.Products, nil
}

func (fe *frontendServer) getProduct(ctx context.Context, id string) (*Product, error) {
	var resp Product
	if err := getJSON(fmt.Sprintf("http://%s/api/products/%s", fe.gatewaySvcAddr, id), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type cartResponse struct {
	UserId string      `json:"userId"`
	Items  []*CartItem `json:"items"`
}

func (fe *frontendServer) getCart(ctx context.Context, userID string) ([]*CartItem, error) {
	reqBody := map[string]string{"userId": userID}
	var resp cartResponse
	if err := postJSON(fmt.Sprintf("http://%s/api/cart/get", fe.gatewaySvcAddr), reqBody, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (fe *frontendServer) emptyCart(ctx context.Context, userID string) error {
	reqBody := map[string]string{"userId": userID}
	return postJSON(fmt.Sprintf("http://%s/api/cart/empty", fe.gatewaySvcAddr), reqBody, nil)
}

func (fe *frontendServer) insertCart(ctx context.Context, userID, productID string, quantity int32) error {
	reqBody := map[string]interface{}{
		"userId": userID,
		"item": map[string]interface{}{
			"productId": productID,
			"quantity":  quantity,
		},
	}
	return postJSON(fmt.Sprintf("http://%s/api/cart/add", fe.gatewaySvcAddr), reqBody, nil)
}

func (fe *frontendServer) convertCurrency(ctx context.Context, m *money.Money, currency string) (*money.Money, error) {
	if avoidNoopCurrencyConversionRPC && m.CurrencyCode == currency {
		return m, nil
	}
	reqBody := map[string]interface{}{
		"from":   m,
		"toCode": currency,
	}
	var result money.Money
	if err := postJSON(fmt.Sprintf("http://%s/api/currency/convert", fe.gatewaySvcAddr), reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type getQuoteResponseFE struct {
	CostUsd *money.Money `json:"costUsd"`
}

func (fe *frontendServer) getShippingQuote(ctx context.Context, items []*CartItem, currency string) (*money.Money, error) {
	reqBody := map[string]interface{}{
		"address": nil,
		"items":   items,
	}
	var resp getQuoteResponseFE
	if err := postJSON(fmt.Sprintf("http://%s/api/shipping/quote", fe.gatewaySvcAddr), reqBody, &resp); err != nil {
		return nil, err
	}
	localized, err := fe.convertCurrency(ctx, resp.CostUsd, currency)
	return localized, errors.Wrap(err, "failed to convert currency for shipping cost")
}

type listRecommendationsResponse struct {
	ProductIds []string `json:"productIds"`
}

func (fe *frontendServer) getRecommendations(ctx context.Context, userID string, productIDs []string) ([]*Product, error) {
	reqBody := map[string]interface{}{
		"userId":     userID,
		"productIds": productIDs,
	}
	var resp listRecommendationsResponse
	if err := postJSON(fmt.Sprintf("http://%s/api/recommendations", fe.gatewaySvcAddr), reqBody, &resp); err != nil {
		return nil, err
	}
	out := make([]*Product, len(resp.ProductIds))
	for i, v := range resp.ProductIds {
		p, err := fe.getProduct(ctx, v)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get recommended product info (#%s)", v)
		}
		out[i] = p
	}
	if len(out) > 4 {
		out = out[:4] // take only first four to fit the UI
	}
	return out, nil
}

type adResponse struct {
	Ads []*Ad `json:"ads"`
}

func (fe *frontendServer) getAd(ctx context.Context, ctxKeys []string) ([]*Ad, error) {
	reqBody := map[string]interface{}{
		"context_keys": ctxKeys,
	}
	var resp adResponse
	if err := postJSON(fmt.Sprintf("http://%s/api/ads", fe.gatewaySvcAddr), reqBody, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to get ads")
	}
	return resp.Ads, nil
}
