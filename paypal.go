package paypal

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	NVP_SANDBOX_URL         = "https://api-3t.sandbox.paypal.com/nvp"
	NVP_PRODUCTION_URL      = "https://api-3t.paypal.com/nvp"
	CHECKOUT_SANDBOX_URL    = "https://www.sandbox.paypal.com/cgi-bin/webscr"
	CHECKOUT_PRODUCTION_URL = "https://www.paypal.com/cgi-bin/webscr"
	NVP_VERSION             = "86"
)

type PayPalClient struct {
	username    string
	password    string
	signature   string
	usesSandbox bool
	client      *http.Client
}

type PayPalDigitalGood struct {
	Name     string
	Amount   float64
	Quantity int16
}

type PaymentInfo struct {
	TransactionId             string
	ParentTransactionId       string
	ReceiptId                 string
	TransactionType           string // can be "cart" or "express-checkout"
	PaymentType               string // can be "none", "echeck", or "instant"
	OrderTime                 string // UTC/GMT
	Amount                    float64
	CurrencyCode              string
	FeeAmount                 float64
	SettleAmount              float64
	TaxAmount                 float64
	ExchangeRate              float64
	PaymentStatus             string   // can be "None", "Cancel-Reversal", "Completed", "Denied", "Expired", "Failed", "In-Progress", "Partially-Refunded", "Pending", "Refunded", "Reversed", "Processed", or "Voided"
	PendingReason             string   // can be "none", "address", "authorization", "echeck", "intl", "multi-currency", "order", "payment-review", "regulatory-review", "unilateral", "verify", or "other"; only returned if PaymentStatus == "Pending"
	ReasonCode                string   // can be "none", "chargeback", "guarantee", "buyer-complaint", "refund", or "other"
	ProtectionEligibility     string   // can be "Eligible", "PartiallyEligible", or "Ineligible"
	ProtectionEligibilityType []string // can be "ItemNotReceivedEligible", "UnauthorizedPaymentEligible", or "Ineligible"
	StoredId                  string
	TerminalId                string
	InstrumentCategory        int    // Possible values are 1, which represents PayPal credit
	InstrumentId              string // Reserved for future use
}

type PayPalResponse struct {
	Ack                string
	CorrelationId      string
	Timestamp          string
	Version            string
	Build              string
	Token              string
	BillingAgreementId string
	ReceiptId          string
	Values             url.Values
	usedSandbox        bool
}

type PayPalSetExpressCheckoutResponse struct {
	PayPalResponse
	Token string
}

type PayPalBillingAgreementResponse struct {
	BillingAgreementId string
	// Should this have a PayPalResponse?
}

type PayPalExpressCheckoutDetails struct {
	PayPalResponse
	Token                          string
	PhoneNumber                    string
	BillingAgreementAcceptedStatus string
	CheckoutStatus                 string
	PayerId                        string
	Email                          string
	PayerStatus                    string
	FirstName                      string
	LastName                       string
	CountryCode                    string
	// Shipping Info

	//Billing Address

	// Transaction Info
}

type PayPalExpressPaymentResponse struct {
	PayPalResponse
	Token                        string
	BillingAgreementId           string
	RedirectRequired             bool
	Note                         string
	MsgSubId                     string
	SuccessPageRedirectRequested bool
	PaymentsInfo                 []PaymentInfo

	// User Options Info

	// Error Info

	// Seller Info

	// Risk Info
}

type PayPalReferenceTransactionResponse struct {
	PaymentInfo

	AvsCode            rune
	Cvv2Match          string
	BillingAgreementId string
	ReceiptId          string
	Values             url.Values
	usedSandbox        bool
	FilterId           int
	FilterName         string
	PaymentAdviceCode  string
	MsgSubId           string
}

type PayPalError struct {
	Ack          string
	ErrorCode    string
	ShortMessage string
	LongMessage  string
	SeverityCode string
}

func (e *PayPalError) Error() string {
	var message string
	if len(e.ErrorCode) != 0 && len(e.ShortMessage) != 0 {
		message = "PayPal Error " + e.ErrorCode + ": " + e.ShortMessage
	} else if len(e.Ack) != 0 {
		message = e.Ack
	} else {
		message = "PayPal is undergoing maintenance.\nPlease try again later."
	}

	return message
}

func (r *PayPalResponse) GetCheckoutUrl() string {
	query := url.Values{}
	query.Set("cmd", "_express-checkout")
	query.Add("token", r.Values["TOKEN"][0])
	checkoutUrl := CHECKOUT_PRODUCTION_URL
	if r.usedSandbox {
		checkoutUrl = CHECKOUT_SANDBOX_URL
	}
	return fmt.Sprintf("%s?%s", checkoutUrl, query.Encode())
}

func SumPayPalDigitalGoodAmounts(goods *[]PayPalDigitalGood) (sum float64) {
	for _, dg := range *goods {
		sum += dg.Amount * float64(dg.Quantity)
	}
	return
}

func NewDefaultClient(username, password, signature string, usesSandbox bool) *PayPalClient {
	return &PayPalClient{username, password, signature, usesSandbox, new(http.Client)}
}

func NewClient(username, password, signature string, usesSandbox bool, client *http.Client) *PayPalClient {
	return &PayPalClient{username, password, signature, usesSandbox, client}
}

func (pClient *PayPalClient) PerformRequest(values url.Values) (*PayPalResponse, error) {
	values.Add("USER", pClient.username)
	values.Add("PWD", pClient.password)
	values.Add("SIGNATURE", pClient.signature)
	values.Add("VERSION", NVP_VERSION)

	endpoint := NVP_PRODUCTION_URL
	if pClient.usesSandbox {
		endpoint = NVP_SANDBOX_URL
	}

	formResponse, err := pClient.client.PostForm(endpoint, values)
	if err != nil {
		return nil, err
	}
	defer formResponse.Body.Close()

	body, err := ioutil.ReadAll(formResponse.Body)
	if err != nil {
		return nil, err
	}

	responseValues, err := url.ParseQuery(string(body))
	response := &PayPalResponse{usedSandbox: pClient.usesSandbox}
	if err == nil {
		response.Ack = responseValues.Get("ACK")
		response.CorrelationId = responseValues.Get("CORRELATIONID")
		response.Timestamp = responseValues.Get("TIMESTAMP")
		response.Version = responseValues.Get("VERSION")
		response.Build = responseValues.Get("2975009")
		response.Token = responseValues.Get("TOKEN")
		response.BillingAgreementId = responseValues.Get("BILLINGAGREEMENTID")
		response.ReceiptId = responseValues.Get("RECEIPTID")
		response.Values = responseValues

		errorCode := responseValues.Get("L_ERRORCODE0")
		if len(errorCode) != 0 || strings.ToLower(response.Ack) == "failure" || strings.ToLower(response.Ack) == "failurewithwarning" {
			pError := new(PayPalError)
			pError.Ack = response.Ack
			pError.ErrorCode = errorCode
			pError.ShortMessage = responseValues.Get("L_SHORTMESSAGE0")
			pError.LongMessage = responseValues.Get("L_LONGMESSAGE0")
			pError.SeverityCode = responseValues.Get("L_SEVERITYCODE0")

			err = pError
		}
	}

	return response, err
}

func (pClient *PayPalClient) SetExpressCheckoutBillingAgreement(paymentAmount float64, currencyCode, billingAgreementDescription, returnUrl, cancelUrl string) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "SetExpressCheckout")
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", paymentAmount))
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", "AUTHORIZATION")
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
	values.Add("RETURNURL", returnUrl)
	values.Add("CANCELURL", cancelUrl)
	values.Add("NOSHIPPING", "1")
	values.Add("REQCONFIRMSHIPPING", "0")
	values.Add("L_BILLINGTYPE0", "MerchantInitiatedBilling")
	values.Add("L_BILLINGAGREEMENTDESCRIPTION0", billingAgreementDescription)

	return pClient.PerformRequest(values)
}

func (pClient *PayPalClient) SetExpressCheckoutDigitalGoods(paymentAmount float64, currencyCode, returnUrl, cancelUrl string, goods []PayPalDigitalGood) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "SetExpressCheckout")
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", paymentAmount))
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", "Sale")
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
	values.Add("RETURNURL", returnUrl)
	values.Add("CANCELURL", cancelUrl)
	values.Add("REQCONFIRMSHIPPING", "0")
	values.Add("NOSHIPPING", "1")
	values.Add("SOLUTIONTYPE", "Sole")

	for i := 0; i < len(goods); i++ {
		good := goods[i]

		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_NAME", i), good.Name)
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_AMT", i), fmt.Sprintf("%.2f", good.Amount))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_QTY", i), fmt.Sprintf("%d", good.Quantity))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_ITEMCATEGORY", i), "Digital")
	}

	return pClient.PerformRequest(values)
}

// Convenience function for Sale (Charge)
func (pClient *PayPalClient) DoExpressCheckoutSale(token, payerId, currencyCode string, finalPaymentAmount float64) (*PayPalResponse, error) {
	return pClient.DoExpressCheckoutPayment(token, payerId, "Sale", currencyCode, finalPaymentAmount)
}

func (pClient *PayPalClient) CreateBillingAgreement(token string) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "CreateBillingAgreement")
	values.Add("TOKEN", token)

	return pClient.PerformRequest(values)
}

func (pClient *PayPalClient) GetExpressCheckoutDetails(token string) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "GetExpressCheckoutDetails")
	values.Add("TOKEN", token)

	return pClient.PerformRequest(values)
}

// paymentType can be "Sale" or "Authorization" or "Order" (ship later)
func (pClient *PayPalClient) DoExpressCheckoutPayment(token, payerId, paymentType, currencyCode string, finalPaymentAmount float64) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "DoExpressCheckoutPayment")
	values.Add("TOKEN", token)
	values.Add("PAYERID", payerId)
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", paymentType)
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", finalPaymentAmount))

	return pClient.PerformRequest(values)
}

// Note that the billingAgreementId must be URL-decoded
func (pClient *PayPalClient) DoReferenceTransaction(billingAgreementId, paymentType string, finalPaymentAmount float64) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "DoReferenceTransaction")
	values.Add("REFERENCEID", billingAgreementId)
	values.Add("PAYMENTACTION", paymentType)
	values.Add("AMT", fmt.Sprintf("%.2f", finalPaymentAmount))

	return pClient.PerformRequest(values)
}
