// Package generated provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/algorand/oapi-codegen DO NOT EDIT.
package generated

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"github.com/algorand/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Get account information.
	// (GET /v2/accounts/{address})
	AccountInformation(ctx echo.Context, address string, params AccountInformationParams) error
	// Get a list of unconfirmed transactions currently in the transaction pool by address.
	// (GET /v2/accounts/{address}/transactions/pending)
	GetPendingTransactionsByAddress(ctx echo.Context, address string, params GetPendingTransactionsByAddressParams) error
	// Get application information.
	// (GET /v2/applications/{application-id})
	GetApplicationByID(ctx echo.Context, applicationId uint64) error
	// Get asset information.
	// (GET /v2/assets/{asset-id})
	GetAssetByID(ctx echo.Context, assetId uint64) error
	// Get the block for the given round.
	// (GET /v2/blocks/{round})
	GetBlock(ctx echo.Context, round uint64, params GetBlockParams) error
	// Get the current supply reported by the ledger.
	// (GET /v2/ledger/supply)
	GetSupply(ctx echo.Context) error
	// Gets the current node status.
	// (GET /v2/status)
	GetStatus(ctx echo.Context) error
	// Gets the node status after waiting for the given round.
	// (GET /v2/status/wait-for-block-after/{round})
	WaitForBlock(ctx echo.Context, round uint64) error
	// Compile TEAL source code to binary, produce its hash
	// (POST /v2/teal/compile)
	TealCompile(ctx echo.Context) error
	// Provide debugging information for a transaction (or group).
	// (POST /v2/teal/dryrun)
	TealDryRun(ctx echo.Context) error
	// Broadcasts a raw transaction to the network.
	// (POST /v2/transactions)
	RawTransaction(ctx echo.Context) error
	// Get parameters for constructing a new transaction
	// (GET /v2/transactions/params)
	TransactionParams(ctx echo.Context) error
	// Get a list of unconfirmed transactions currently in the transaction pool.
	// (GET /v2/transactions/pending)
	GetPendingTransactions(ctx echo.Context, params GetPendingTransactionsParams) error
	// Get a specific pending transaction.
	// (GET /v2/transactions/pending/{txid})
	PendingTransactionInformation(ctx echo.Context, txid string, params PendingTransactionInformationParams) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// AccountInformation converts echo context to params.
func (w *ServerInterfaceWrapper) AccountInformation(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
		"format": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params AccountInformationParams
	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.AccountInformation(ctx, address, params)
	return err
}

// GetPendingTransactionsByAddress converts echo context to params.
func (w *ServerInterfaceWrapper) GetPendingTransactionsByAddress(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
		"max":    true,
		"format": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetPendingTransactionsByAddressParams
	// ------------- Optional query parameter "max" -------------
	if paramValue := ctx.QueryParam("max"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max", ctx.QueryParams(), &params.Max)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max: %s", err))
	}

	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPendingTransactionsByAddress(ctx, address, params)
	return err
}

// GetApplicationByID converts echo context to params.
func (w *ServerInterfaceWrapper) GetApplicationByID(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "application-id" -------------
	var applicationId uint64

	err = runtime.BindStyledParameter("simple", false, "application-id", ctx.Param("application-id"), &applicationId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter application-id: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetApplicationByID(ctx, applicationId)
	return err
}

// GetAssetByID converts echo context to params.
func (w *ServerInterfaceWrapper) GetAssetByID(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "asset-id" -------------
	var assetId uint64

	err = runtime.BindStyledParameter("simple", false, "asset-id", ctx.Param("asset-id"), &assetId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset-id: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetAssetByID(ctx, assetId)
	return err
}

// GetBlock converts echo context to params.
func (w *ServerInterfaceWrapper) GetBlock(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
		"format": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "round" -------------
	var round uint64

	err = runtime.BindStyledParameter("simple", false, "round", ctx.Param("round"), &round)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetBlockParams
	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetBlock(ctx, round, params)
	return err
}

// GetSupply converts echo context to params.
func (w *ServerInterfaceWrapper) GetSupply(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetSupply(ctx)
	return err
}

// GetStatus converts echo context to params.
func (w *ServerInterfaceWrapper) GetStatus(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetStatus(ctx)
	return err
}

// WaitForBlock converts echo context to params.
func (w *ServerInterfaceWrapper) WaitForBlock(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "round" -------------
	var round uint64

	err = runtime.BindStyledParameter("simple", false, "round", ctx.Param("round"), &round)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter round: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.WaitForBlock(ctx, round)
	return err
}

// TealCompile converts echo context to params.
func (w *ServerInterfaceWrapper) TealCompile(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.TealCompile(ctx)
	return err
}

// TealDryRun converts echo context to params.
func (w *ServerInterfaceWrapper) TealDryRun(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.TealDryRun(ctx)
	return err
}

// RawTransaction converts echo context to params.
func (w *ServerInterfaceWrapper) RawTransaction(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.RawTransaction(ctx)
	return err
}

// TransactionParams converts echo context to params.
func (w *ServerInterfaceWrapper) TransactionParams(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.TransactionParams(ctx)
	return err
}

// GetPendingTransactions converts echo context to params.
func (w *ServerInterfaceWrapper) GetPendingTransactions(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
		"max":    true,
		"format": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error

	ctx.Set("api_key.Scopes", []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetPendingTransactionsParams
	// ------------- Optional query parameter "max" -------------
	if paramValue := ctx.QueryParam("max"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "max", ctx.QueryParams(), &params.Max)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max: %s", err))
	}

	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPendingTransactions(ctx, params)
	return err
}

// PendingTransactionInformation converts echo context to params.
func (w *ServerInterfaceWrapper) PendingTransactionInformation(ctx echo.Context) error {

	validQueryParams := map[string]bool{
		"pretty": true,
		"format": true,
	}

	// Check for unknown query parameters.
	for name, _ := range ctx.QueryParams() {
		if _, ok := validQueryParams[name]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unknown parameter detected: %s", name))
		}
	}

	var err error
	// ------------- Path parameter "txid" -------------
	var txid string

	err = runtime.BindStyledParameter("simple", false, "txid", ctx.Param("txid"), &txid)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter txid: %s", err))
	}

	ctx.Set("api_key.Scopes", []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params PendingTransactionInformationParams
	// ------------- Optional query parameter "format" -------------
	if paramValue := ctx.QueryParam("format"); paramValue != "" {

	}

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.PendingTransactionInformation(ctx, txid, params)
	return err
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}, si ServerInterface, m ...echo.MiddlewareFunc) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET("/v2/accounts/:address", wrapper.AccountInformation, m...)
	router.GET("/v2/accounts/:address/transactions/pending", wrapper.GetPendingTransactionsByAddress, m...)
	router.GET("/v2/applications/:application-id", wrapper.GetApplicationByID, m...)
	router.GET("/v2/assets/:asset-id", wrapper.GetAssetByID, m...)
	router.GET("/v2/blocks/:round", wrapper.GetBlock, m...)
	router.GET("/v2/ledger/supply", wrapper.GetSupply, m...)
	router.GET("/v2/status", wrapper.GetStatus, m...)
	router.GET("/v2/status/wait-for-block-after/:round", wrapper.WaitForBlock, m...)
	router.POST("/v2/teal/compile", wrapper.TealCompile, m...)
	router.POST("/v2/teal/dryrun", wrapper.TealDryRun, m...)
	router.POST("/v2/transactions", wrapper.RawTransaction, m...)
	router.GET("/v2/transactions/params", wrapper.TransactionParams, m...)
	router.GET("/v2/transactions/pending", wrapper.GetPendingTransactions, m...)
	router.GET("/v2/transactions/pending/:txid", wrapper.PendingTransactionInformation, m...)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file.
func GetSwagger() (*openapi3.Swagger, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error loading Swagger: %s", err)
	}
	return swagger, nil
}
