package billing

import (
	"context"
	"errors"
	"testing"

	domainbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/billing"
	paymentport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/payment"
)

type nanoCheckoutProviderStub struct {
	quote  paymentport.NanoCheckoutInput
	verify paymentport.NanoVerifyInput
	calls  int
}

func (s *nanoCheckoutProviderStub) Quote(_ context.Context, input paymentport.NanoCheckoutInput) (paymentport.NanoCheckout, error) {
	s.quote = input
	return paymentport.NanoCheckout{Network: "nano:mainnet", Scheme: "exact", AmountRaw: "1"}, nil
}

func (s *nanoCheckoutProviderStub) VerifyPayment(_ context.Context, input paymentport.NanoVerifyInput) (string, error) {
	s.verify = input
	s.calls++
	return input.BlockHash, nil
}

func pendingNanoOrder() *domainbilling.PaymentOrder {
	return &domainbilling.PaymentOrder{
		OrderNo:        "order-123",
		OrderType:      domainbilling.PaymentOrderTypeTopUp,
		Provider:       domainbilling.PaymentProviderNano,
		Status:         domainbilling.PaymentStatusPending,
		PayCurrency:    "USD",
		PayAmountCents: 1250,
	}
}

func TestNanoCheckoutIsOffUnlessConfigured(t *testing.T) {
	service := NewPaymentCheckoutService(nil, nil)
	if _, err := service.CreateNanoCheckout(t.Context(), NanoCheckoutInput{Order: pendingNanoOrder()}); !errors.Is(err, ErrPaymentProviderUnavailable) {
		t.Fatalf("CreateNanoCheckout without nano: %v", err)
	}
	if _, err := service.VerifyNanoPayment(t.Context(), NanoVerifyInput{Order: pendingNanoOrder()}); !errors.Is(err, ErrPaymentProviderUnavailable) {
		t.Fatalf("VerifyNanoPayment without nano: %v", err)
	}
}

func TestCreateNanoCheckoutMapsOrderAtApplicationBoundary(t *testing.T) {
	provider := &nanoCheckoutProviderStub{}
	service := NewPaymentCheckoutService(nil, nil).WithNano(provider)
	if _, err := service.CreateNanoCheckout(t.Context(), NanoCheckoutInput{
		PayTo: "nano_pay_to", XNOPerUnit: "2.5", Order: pendingNanoOrder(),
	}); err != nil {
		t.Fatalf("CreateNanoCheckout() error = %v", err)
	}
	if provider.quote.OrderNo != "order-123" || provider.quote.PayAmountCents != 1250 ||
		provider.quote.PayTo != "nano_pay_to" || provider.quote.XNOPerUnit != "2.5" {
		t.Fatalf("unexpected quote mapping: %#v", provider.quote)
	}

	epayOrder := pendingNanoOrder()
	epayOrder.Provider = domainbilling.PaymentProviderEPay
	if _, err := service.CreateNanoCheckout(t.Context(), NanoCheckoutInput{Order: epayOrder}); !errors.Is(err, ErrPaymentOrderStateInvalid) {
		t.Fatalf("an epay order got a nano quote: %v", err)
	}
}

func TestVerifyNanoPaymentRefusesSettledOrders(t *testing.T) {
	provider := &nanoCheckoutProviderStub{}
	service := NewPaymentCheckoutService(nil, nil).WithNano(provider)

	hash, err := service.VerifyNanoPayment(t.Context(), NanoVerifyInput{
		NodeURL: "https://node.example", PayTo: "nano_pay_to", XNOPerUnit: "2.5", Order: pendingNanoOrder(), BlockHash: "ABC",
	})
	if err != nil || hash != "ABC" || provider.verify.OrderNo != "order-123" || provider.verify.PayAmountCents != 1250 ||
		provider.verify.NodeURL != "https://node.example" {
		t.Fatalf("VerifyNanoPayment = %q, %v, input %#v", hash, err, provider.verify)
	}

	// The stored order is what refuses a replay, so it holds across a restart.
	paid := pendingNanoOrder()
	paid.Status = domainbilling.PaymentStatusPaid
	recorded := pendingNanoOrder()
	recorded.ExternalPaymentID = "ABC"
	stripeOrder := pendingNanoOrder()
	stripeOrder.Provider = domainbilling.PaymentProviderStripe
	for name, order := range map[string]*domainbilling.PaymentOrder{"paid": paid, "recorded": recorded, "stripe": stripeOrder} {
		if _, err := service.VerifyNanoPayment(t.Context(), NanoVerifyInput{Order: order, BlockHash: "ABC"}); !errors.Is(err, ErrPaymentOrderStateInvalid) {
			t.Errorf("%s order: err = %v", name, err)
		}
	}
	if provider.calls != 1 {
		t.Fatalf("the node was asked %d times, want 1", provider.calls)
	}
}
