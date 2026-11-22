package billing

import (
	"context"
	"fmt"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/textutil"
	"net/url"

	domainbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/billing"
	paymentport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/payment"
)

type stripeCheckoutProvider interface {
	CreateCheckoutSession(ctx context.Context, input paymentport.StripeCheckoutInput) (paymentport.CheckoutResult, error)
}

type epayCheckoutProvider interface {
	CreateCheckout(ctx context.Context, input paymentport.EPayCheckoutInput) (paymentport.CheckoutResult, error)
	VerifySignature(values url.Values, key string) bool
}

type nanoCheckoutProvider interface {
	Quote(ctx context.Context, input paymentport.NanoCheckoutInput) (paymentport.NanoCheckout, error)
	VerifyPayment(ctx context.Context, input paymentport.NanoVerifyInput) (string, error)
}

// PaymentCheckoutService 编排支付收银台创建和第三方协议映射。
type PaymentCheckoutService struct {
	stripe stripeCheckoutProvider
	epay   epayCheckoutProvider
	nano   nanoCheckoutProvider
}

// NanoCheckoutInput 定义创建 Nano (XNO) 收款报价的应用层入参。
type NanoCheckoutInput struct {
	PayTo      string
	XNOPerUnit string
	Order      *domainbilling.PaymentOrder
}

// NanoVerifyInput 定义校验 Nano (XNO) 付款的应用层入参。
type NanoVerifyInput struct {
	NodeURL    string
	PayTo      string
	XNOPerUnit string
	Order      *domainbilling.PaymentOrder
	BlockHash  string
}

// StripeCheckoutInput 定义创建 Stripe Checkout Session 的应用层入参。
type StripeCheckoutInput struct {
	SecretKey  string
	SuccessURL string
	CancelURL  string
	Order      *domainbilling.PaymentOrder
	Plan       *domainbilling.Plan
}

// EPayCheckoutInput 定义创建易支付 submit.php 页面跳转收银台的应用层入参。
type EPayCheckoutInput struct {
	GatewayURL  string
	MerchantID  string
	MerchantKey string
	PaymentType string
	NotifyURL   string
	ReturnURL   string
	Order       *domainbilling.PaymentOrder
	Plan        *domainbilling.Plan
}

// PaymentCheckoutResult 表示第三方支付收银台标识与跳转地址。
type PaymentCheckoutResult struct {
	ID  string
	URL string
}

// PaymentProduct 表示支付渠道展示的商品名称和说明。
type PaymentProduct struct {
	Name        string
	Description string
}

// NewPaymentCheckoutService 创建依赖完整的支付收银台应用服务。
func NewPaymentCheckoutService(stripe stripeCheckoutProvider, epay epayCheckoutProvider) *PaymentCheckoutService {
	return &PaymentCheckoutService{stripe: stripe, epay: epay}
}

// WithNano 启用 Nano (XNO) 收款渠道；未调用时 Nano 相关方法返回 ErrPaymentProviderUnavailable。
func (s *PaymentCheckoutService) WithNano(nano nanoCheckoutProvider) *PaymentCheckoutService {
	if s != nil {
		s.nano = nano
	}
	return s
}

// CreateNanoCheckout 为订单生成唯一可接受的 Nano 付款：精确 raw 金额与绑定订单的代表账户。
func (s *PaymentCheckoutService) CreateNanoCheckout(ctx context.Context, input NanoCheckoutInput) (paymentport.NanoCheckout, error) {
	if s == nil || s.nano == nil {
		return paymentport.NanoCheckout{}, ErrPaymentProviderUnavailable
	}
	if input.Order == nil {
		return paymentport.NanoCheckout{}, fmt.Errorf("payment order is required")
	}
	if input.Order.Provider != domainbilling.PaymentProviderNano || input.Order.Status != domainbilling.PaymentStatusPending {
		return paymentport.NanoCheckout{}, ErrPaymentOrderStateInvalid
	}
	return s.nano.Quote(ctx, paymentport.NanoCheckoutInput{
		PayTo:          input.PayTo,
		XNOPerUnit:     input.XNOPerUnit,
		OrderNo:        input.Order.OrderNo,
		PayCurrency:    input.Order.PayCurrency,
		PayAmountCents: input.Order.PayAmountCents,
	})
}

// VerifyNanoPayment 从运营方节点读取区块并校验其精确支付该订单，返回作为外部支付 ID 保存的区块哈希。
// 已支付或已记录外部支付 ID 的订单一律拒绝，因此同一区块在重启后也不能再次入账；
// 代表账户承诺了订单号，区块也无法支付其他订单。
func (s *PaymentCheckoutService) VerifyNanoPayment(ctx context.Context, input NanoVerifyInput) (string, error) {
	if s == nil || s.nano == nil {
		return "", ErrPaymentProviderUnavailable
	}
	if input.Order == nil {
		return "", fmt.Errorf("payment order is required")
	}
	if input.Order.Provider != domainbilling.PaymentProviderNano ||
		input.Order.Status != domainbilling.PaymentStatusPending ||
		input.Order.ExternalPaymentID != "" {
		return "", ErrPaymentOrderStateInvalid
	}
	return s.nano.VerifyPayment(ctx, paymentport.NanoVerifyInput{
		NodeURL:        input.NodeURL,
		PayTo:          input.PayTo,
		XNOPerUnit:     input.XNOPerUnit,
		OrderNo:        input.Order.OrderNo,
		PayAmountCents: input.Order.PayAmountCents,
		BlockHash:      input.BlockHash,
	})
}

// VerifyEPaySignature 校验易支付 submit.php 通知签名。
func (s *PaymentCheckoutService) VerifyEPaySignature(values url.Values, key string) bool {
	return s != nil && s.epay != nil && s.epay.VerifySignature(values, key)
}

// CreateEPayCheckout 编排易支付 submit.php 页面跳转收银台，并将领域订单映射为协议字段。
func (s *PaymentCheckoutService) CreateEPayCheckout(ctx context.Context, input EPayCheckoutInput) (PaymentCheckoutResult, error) {
	if s == nil || s.epay == nil {
		return PaymentCheckoutResult{}, ErrPaymentProviderUnavailable
	}
	if input.Order == nil {
		return PaymentCheckoutResult{}, fmt.Errorf("payment order is required")
	}
	product := DescribePaymentProduct(input.Order, input.Plan)
	result, err := s.epay.CreateCheckout(ctx, paymentport.EPayCheckoutInput{
		GatewayURL:     input.GatewayURL,
		MerchantID:     input.MerchantID,
		MerchantKey:    input.MerchantKey,
		PaymentType:    input.PaymentType,
		OrderNo:        input.Order.OrderNo,
		NotifyURL:      input.NotifyURL,
		ReturnURL:      input.ReturnURL,
		PayCurrency:    input.Order.PayCurrency,
		PayAmountCents: input.Order.PayAmountCents,
		ProductName:    product.Name,
	})
	if err != nil {
		return PaymentCheckoutResult{}, err
	}
	return PaymentCheckoutResult{URL: result.URL}, nil
}

// CreateStripeCheckoutSession 编排 Stripe 收银台创建，并将领域订单映射为第三方协议字段。
func (s *PaymentCheckoutService) CreateStripeCheckoutSession(ctx context.Context, input StripeCheckoutInput) (PaymentCheckoutResult, error) {
	if s == nil || s.stripe == nil {
		return PaymentCheckoutResult{}, ErrPaymentProviderUnavailable
	}
	if input.Order == nil {
		return PaymentCheckoutResult{}, fmt.Errorf("payment order is required")
	}
	product := DescribePaymentProduct(input.Order, input.Plan)
	result, err := s.stripe.CreateCheckoutSession(ctx, paymentport.StripeCheckoutInput{
		SecretKey:          input.SecretKey,
		SuccessURL:         input.SuccessURL,
		CancelURL:          input.CancelURL,
		OrderNo:            input.Order.OrderNo,
		OrderType:          input.Order.OrderType,
		UserID:             input.Order.UserID,
		BaseCurrency:       input.Order.BaseCurrency,
		BaseAmountCents:    input.Order.BaseAmountCents,
		PayCurrency:        input.Order.PayCurrency,
		PayAmountCents:     input.Order.PayAmountCents,
		FXRate:             input.Order.FXRate,
		ProductName:        product.Name,
		ProductDescription: product.Description,
	})
	if err != nil {
		return PaymentCheckoutResult{}, err
	}
	return PaymentCheckoutResult{ID: result.ID, URL: result.URL}, nil
}

// DescribePaymentProduct 统一生成各支付渠道使用的商品展示信息。
func DescribePaymentProduct(order *domainbilling.PaymentOrder, plan *domainbilling.Plan) PaymentProduct {
	if order != nil && order.OrderType == domainbilling.PaymentOrderTypeTopUp {
		amountCents := order.PayAmountCents
		if amountCents <= 0 {
			amountCents = order.BaseAmountCents
		}
		return PaymentProduct{
			Name:        "按量余额充值",
			Description: fmt.Sprintf("充值 %s %.2f 至按量余额", textutil.FirstNonEmpty(order.PayCurrency, order.BaseCurrency, "USD"), float64(amountCents)/100),
		}
	}
	if plan != nil {
		return PaymentProduct{
			Name:        textutil.FirstNonEmpty(plan.Name, plan.Code),
			Description: textutil.FirstNonEmpty(plan.Description, plan.Code),
		}
	}
	return PaymentProduct{Name: "订阅方案", Description: "订阅方案支付"}
}
