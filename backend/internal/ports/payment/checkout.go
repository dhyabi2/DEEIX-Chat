// Package payment 定义支付网关收银台端口的数据契约。
package payment

// StripeCheckoutInput 定义 Stripe Checkout Session 所需的协议字段。
type StripeCheckoutInput struct {
	SecretKey          string
	SuccessURL         string
	CancelURL          string
	OrderNo            string
	OrderType          string
	UserID             uint
	BaseCurrency       string
	BaseAmountCents    int64
	PayCurrency        string
	PayAmountCents     int64
	FXRate             string
	ProductName        string
	ProductDescription string
}

// EPayCheckoutInput 定义易支付 submit.php 页面跳转所需的协议字段。
type EPayCheckoutInput struct {
	GatewayURL     string
	MerchantID     string
	MerchantKey    string
	PaymentType    string
	OrderNo        string
	NotifyURL      string
	ReturnURL      string
	PayCurrency    string
	PayAmountCents int64
	ProductName    string
}

// NanoCheckoutInput 定义 Nano (XNO) nano:mainnet exact 收款报价所需的协议字段。
// XNOPerUnit 是运营方配置的 1 个计价货币单位对应的 XNO 价格（十进制字符串，例如 "2.5"）。
type NanoCheckoutInput struct {
	PayTo          string
	XNOPerUnit     string
	OrderNo        string
	PayCurrency    string
	PayAmountCents int64
}

// NanoCheckout 表示一笔订单唯一接受的 Nano 付款：收款账户、精确 raw 金额，
// 以及把付款绑定到该订单的代表账户（representative）。
type NanoCheckout struct {
	Network        string
	Scheme         string
	PayTo          string
	AmountRaw      string
	Representative string
	PaymentURI     string
}

// NanoVerifyInput 定义校验一笔 Nano 付款所需的协议字段。NodeURL 由运营方配置，只读。
type NanoVerifyInput struct {
	NodeURL        string
	PayTo          string
	XNOPerUnit     string
	OrderNo        string
	PayAmountCents int64
	BlockHash      string
}

// CheckoutResult 表示支付网关返回的收银台标识与跳转地址。
type CheckoutResult struct {
	ID  string
	URL string
}
