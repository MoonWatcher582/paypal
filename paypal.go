package paypal

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
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
	StoreId                   string
	TerminalId                string
	InstrumentCategory        int    // Possible values are 1, which represents PayPal credit
	InstrumentId              string // Reserved for future use
}

type AddressInfo struct {
	Name              string
	Street            string
	Street2           string
	City              string
	State             string
	Zip               string
	CountryCode       string
	Country           string
	PhoneNumber       string
	Status            string // none, Confirmed, or Unconfirmed
	NormatilzedStatus string // For Brazil only: none, Normalized, Unnormalized, or UserPrefered
}

type PayPalResponse struct {
	Ack           string
	CorrelationId string
	Timestamp     string
	Version       string
	Build         string
	Values        url.Values
	usedSandbox   bool
}

type PayPalSetExpressCheckoutResponse struct {
	PayPalResponse

	Token string
}

type PayPalBillingAgreementResponse struct {
	PayPalResponse

	BillingAgreementId string
}

type PayPalExpressCheckoutDetails struct {
	PayPalResponse

	Token                    string
	PhoneNumber              string
	BillingAgreementAccepted bool
	CheckoutStatus           string
	PayerID                  string
	Email                    string
	PayerStatusVerified      bool
	FirstName                string
	LastName                 string
	CountryCode              string
	ShippingAddresses        []AddressInfo
	PaymentsInfo             []PaymentInfo
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
	PayPalResponse
	PaymentInfo

	AvsCode            string
	Cvv2Match          string
	BillingAgreementId string
	PaymentAdviceCode  string
	MsgSubId           string
}

type PayPalRefundTransactionResponse struct {
	PayPalResponse

	RefundTransactionId string
	RefundFeeAmount     float64 // 2.9% of the refund + $.30
	GrossRefundAmount   float64 // Amount refunded from this request
	NetRefundAmount     float64 // GrossRefundAmt - RefundFeeAmt
	TotalRefundAmount   float64 // Total amount refunded for this transaction
	CurrencyCode        string
	RefundStatus        string // instant, delayed, or none if transaction fails
	PendingReason       string // none, echeck, or regulatoryreview
	MsgSubId            string
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

func (pClient *PayPalClient) SetExpressCheckoutBillingAgreement(paymentAmount float64, currencyCode, billingAgreementDescription, returnUrl, cancelUrl string) (*PayPalSetExpressCheckoutResponse, error) {
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

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	return &PayPalSetExpressCheckoutResponse{
		PayPalResponse: *resp,
		Token:          resp.Values.Get("TOKEN"),
	}, nil
}

func (pClient *PayPalClient) SetExpressCheckoutDigitalGoods(paymentAmount float64, currencyCode, returnUrl, cancelUrl string, goods []PayPalDigitalGood) (*PayPalSetExpressCheckoutResponse, error) {
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

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	return &PayPalSetExpressCheckoutResponse{
		PayPalResponse: *resp,
		Token:          resp.Values.Get("TOKEN"),
	}, nil
}

func (pClient *PayPalClient) CreateBillingAgreement(token string) (*PayPalBillingAgreementResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "CreateBillingAgreement")
	values.Add("TOKEN", token)

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	return &PayPalBillingAgreementResponse{
		PayPalResponse:     *resp,
		BillingAgreementId: resp.Values.Get("BILLINGAGREEMENTID"),
	}, nil
}

func (pClient *PayPalClient) GetExpressCheckoutDetails(token string) (*PayPalExpressCheckoutDetails, error) {
	values := url.Values{}
	values.Set("METHOD", "GetExpressCheckoutDetails")
	values.Add("TOKEN", token)

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	// PaymentsInfo                   []PaymentInfo

	r := &PayPalExpressCheckoutDetails{
		PayPalResponse: *resp,
		Token:          resp.Values.Get("TOKEN"),
		PhoneNumber:    resp.Values.Get("PHONENUM"),
		CheckoutStatus: resp.Values.Get("CHECKOUTSTATUS"),
		PayerID:        resp.Values.Get("PAYERID"),
		Email:          resp.Values.Get("EMAIL"),
		FirstName:      resp.Values.Get("FIRSTNAME"),
		LastName:       resp.Values.Get("LASTNAME"),
		CountryCode:    resp.Values.Get("COUNTRYCODE"),
	}

	if resp.Values.Get("BILLINGAGREEMENTACCEPTEDSTATUS") == "1" {
		r.BillingAgreementAccepted = true
	}

	if resp.Values.Get("PAYERSTATUS") == "verified" {
		r.PayerStatusVerified = true
	}

	r.ShippingAddresses = make([]AddressInfo, 10)
	for i := 0; i < 10; i++ {
		idx := fmt.Sprintf("%d", i)
		address := AddressInfo{
			Name:              resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTONAME"),
			Street:            resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOSTREET"),
			Street2:           resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOSTREET2"),
			City:              resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOCITY"),
			State:             resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOSTATE"),
			Zip:               resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOZIP"),
			CountryCode:       resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOCOUNTRYCODE"),
			Country:           resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOCOUNTRYNAME"),
			PhoneNumber:       resp.Values.Get("PAYMENTREQUEST_" + idx + "_SHIPTOPHONENUM"),
			Status:            resp.Values.Get("PAYMENTREQUEST_" + idx + "_ADDRESSSTATUS"),
			NormatilzedStatus: resp.Values.Get("PAYMENTREQUEST_" + idx + "_ADDRESSNORMALIZATIONSTATUS"),
		}
		r.ShippingAddresses = append(r.ShippingAddresses, address)
	}

	return r, nil
}

// // paymentType can be "Sale" or "Authorization" or "Order" (ship later)
// func (pClient *PayPalClient) DoExpressCheckoutPayment(token, payerID, paymentType, currencyCode string, finalPaymentAmount float64) (*PayPalExpressPaymentResponse, error) {
// 	values := url.Values{}
// 	values.Set("METHOD", "DoExpressCheckoutPayment")
// 	values.Add("TOKEN", token)
// 	values.Add("PAYERID", payerID)
// 	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", paymentType)
// 	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
// 	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", finalPaymentAmount))

// 	return pClient.PerformRequest(values)
// }

// Note that the billingAgreementId must be URL-decoded
func (pClient *PayPalClient) DoReferenceTransaction(billingAgreementId, paymentType string, finalPaymentAmount float64) (*PayPalReferenceTransactionResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "DoReferenceTransaction")
	values.Add("REFERENCEID", billingAgreementId)
	values.Add("PAYMENTACTION", paymentType)
	values.Add("AMT", fmt.Sprintf("%.2f", finalPaymentAmount))

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	amt, _ := strconv.ParseFloat(resp.Values.Get("AMT"), 64)
	feeAmt, _ := strconv.ParseFloat(resp.Values.Get("FEEAMT"), 64)
	settleAmt, _ := strconv.ParseFloat(resp.Values.Get("SETTLEAMT"), 64)
	taxAmt, _ := strconv.ParseFloat(resp.Values.Get("TAXAMT"), 64)
	exchangeRate, _ := strconv.ParseFloat(resp.Values.Get("EXCHANGERATE"), 64)

	instrumentCategory, _ := strconv.Atoi(resp.Values.Get("INSTRUMENTCATEGORY"))

	protectionEligibilityTypes := strings.Split(resp.Values.Get("PROTECTIONELIGIBILITYTYPE"), ",")

	return &PayPalReferenceTransactionResponse{
		PayPalResponse:     *resp,
		AvsCode:            resp.Values.Get("AVSCODE"),
		Cvv2Match:          resp.Values.Get("CVV2MATCH"),
		BillingAgreementId: resp.Values.Get("BILLINGAGREEMENTID"),
		PaymentAdviceCode:  resp.Values.Get("PAYMENTADVICECODE"),
		MsgSubId:           resp.Values.Get("MSGSUBID"),
		PaymentInfo: PaymentInfo{
			TransactionId:             resp.Values.Get("TRANSACTIONID"),
			ParentTransactionId:       resp.Values.Get("PARENTTRANSACTIONID"),
			ReceiptId:                 resp.Values.Get("RECEIPTID"),
			TransactionType:           resp.Values.Get("TRANSACTIONTYPE"),
			PaymentType:               resp.Values.Get("PAYMENTTYPE"),
			OrderTime:                 resp.Values.Get("ORDERTIME"),
			Amount:                    amt,
			CurrencyCode:              resp.Values.Get("CURRENCYCODE"),
			FeeAmount:                 feeAmt,
			SettleAmount:              settleAmt,
			TaxAmount:                 taxAmt,
			ExchangeRate:              exchangeRate,
			PaymentStatus:             resp.Values.Get("PAYMENTSTATUS"),
			PendingReason:             resp.Values.Get("PENDINGREASON"),
			ReasonCode:                resp.Values.Get("REASONCODE"),
			ProtectionEligibility:     resp.Values.Get("PROTECTIONELIGIBILITY"),
			ProtectionEligibilityType: protectionEligibilityTypes,
			StoreId:                   resp.Values.Get("STOREID"),
			TerminalId:                resp.Values.Get("TERMINALID"),
			InstrumentCategory:        instrumentCategory,
			InstrumentId:              resp.Values.Get("INSTRUMENTID"),
		},
	}, nil
}

// Point-of-Sale transactions not supported currently
func (pClient *PayPalClient) RefundTransaction(refundAmount, shippingAmount, taxAmount float64, transactionId, invoiceId, msgSubId, currencyCode string, partialRefund bool) (*PayPalRefundTransactionResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "RefundTransaction")
	values.Add("TRANSACTIONID", transactionId)
	values.Add("INVOICEID", invoiceId)
	values.Add("SHIPPINGAMT", fmt.Sprintf("%.2f", shippingAmount))
	values.Add("TAXAMT", fmt.Sprintf("%.2f", taxAmount))
	values.Add("MSGSUBID", msgSubId)

	refundType := "Full"
	if partialRefund {
		refundType = "Partial"
	}
	values.Add("REFUNDTYPE", refundType)

	if currencyCode != "" {
		values.Add("CURRENCYCODE", currencyCode)
	}

	if partialRefund {
		values.Add("AMT", fmt.Sprintf("%.2f", refundAmount))
	}

	resp, err := pClient.PerformRequest(values)
	if err != nil {
		return nil, err
	}

	refundFee, _ := strconv.ParseFloat(resp.Values.Get("FEEREFUNDAMT"), 64)
	netRefund, _ := strconv.ParseFloat(resp.Values.Get("NETREFUNDAMT"), 64)
	grossRefund, _ := strconv.ParseFloat(resp.Values.Get("GROSSREFUNDAMT"), 64)
	totalRefund, _ := strconv.ParseFloat(resp.Values.Get("TOTALREFUNDAMT"), 64)

	return &PayPalRefundTransactionResponse{
		PayPalResponse:      *resp,
		RefundTransactionId: resp.Values.Get("REFUNDTRANSACTIONID"),
		RefundFeeAmount:     refundFee,
		NetRefundAmount:     netRefund,
		GrossRefundAmount:   grossRefund,
		TotalRefundAmount:   totalRefund,
		CurrencyCode:        resp.Values.Get("CURRENCYCODE"),
		RefundStatus:        resp.Values.Get("REFUNDSTATUS"),
		PendingReason:       resp.Values.Get("PENDINGREASON"),
		MsgSubId:            resp.Values.Get("MSGSUBID"),
	}, nil
}

// MassPay only returns a standard response
// Only supports one transaction per request currently
func (pClient *PayPalClient) MassPay(paymentAmount float64, emailSubject, currencyCode, trackingId, note, receiverType, identifier string) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "MassPay")
	values.Add("EMAILSUBJECT", emailSubject)
	values.Add("CURRENCYCODE", currencyCode)
	values.Add("L_AMT0", fmt.Sprintf("%.2f", paymentAmount))
	values.Add("L_UNIQUEID0", trackingId)
	values.Add("L_NOTE0", note)

	switch receiverType {
	case "EmailAddress":
		values.Add("L_EMAIL0", identifier)
	case "UserId":
		values.Add("L_RECEIVERID0", identifier)
	case "PhoneNumber":
		values.Add("L_RECEIVERPHONE0", identifier)
	default:
		return nil, &PayPalError{
			ShortMessage: "Invalid receiver type for mass pay! Must be UserId, EmailAddress, or PhoneNumber",
		}
	}

	values.Add("RECEIVERTYPE", receiverType)

	return pClient.PerformRequest(values)
}
