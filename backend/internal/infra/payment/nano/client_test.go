package nano

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"

	paymentport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/payment"
)

func TestClientQuoteBindsTheOrder(t *testing.T) {
	payTo := account(t, 0x22)
	quote, err := New(nil).Quote(context.Background(), paymentport.NanoCheckoutInput{
		PayTo:          payTo,
		XNOPerUnit:     "2.5",
		OrderNo:        "order-123",
		PayCurrency:    "USD",
		PayAmountCents: 1250,
	})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	// 12.50 units at 2.5 XNO a unit is 31.25 XNO, derived from 10^30 raw.
	want, _ := RawFromXNO("31.25")
	if quote.AmountRaw != want.String() || quote.Network != Network || quote.Scheme != Scheme || quote.PayTo != payTo {
		t.Fatalf("quote = %+v, want raw %s", quote, want)
	}
	if quote.Representative != CommitmentAccount("order-123", payTo, want.String()) {
		t.Fatalf("representative %s does not commit to the order", quote.Representative)
	}
	u, err := url.Parse(quote.PaymentURI)
	if err != nil || u.Scheme != "nano" || u.Opaque != payTo || u.Query().Get("amount") != want.String() ||
		u.Query().Get("representative") != quote.Representative {
		t.Fatalf("payment URI = %q", quote.PaymentURI)
	}

	other, _ := New(nil).Quote(context.Background(), paymentport.NanoCheckoutInput{
		PayTo: payTo, XNOPerUnit: "2.5", OrderNo: "order-124", PayAmountCents: 1250,
	})
	if other.Representative == quote.Representative {
		t.Fatal("two orders share one binding")
	}
}

func TestClientQuoteRefusesBadConfiguration(t *testing.T) {
	payTo := account(t, 0x22)
	cases := map[string]paymentport.NanoCheckoutInput{
		"bad checksum": {PayTo: payTo[:len(payTo)-1] + "1", XNOPerUnit: "2.5", OrderNo: "o", PayAmountCents: 100},
		"no account":   {XNOPerUnit: "2.5", OrderNo: "o", PayAmountCents: 100},
		"no rate":      {PayTo: payTo, OrderNo: "o", PayAmountCents: 100},
		"float rate":   {PayTo: payTo, XNOPerUnit: "1e3", OrderNo: "o", PayAmountCents: 100},
		"zero amount":  {PayTo: payTo, XNOPerUnit: "2.5", OrderNo: "o"},
		"no order":     {PayTo: payTo, XNOPerUnit: "2.5", PayAmountCents: 100},
	}
	for name, input := range cases {
		if _, err := New(nil).Quote(context.Background(), input); err == nil {
			t.Errorf("%s: quote accepted", name)
		}
	}
}

func TestClientVerifyPaymentReadsTheNode(t *testing.T) {
	payer, payTo := account(t, 0x11), account(t, 0x22)
	quote, err := New(nil).Quote(context.Background(), paymentport.NanoCheckoutInput{
		PayTo: payTo, XNOPerUnit: "2.5", OrderNo: "order-123", PayAmountCents: 1250,
	})
	if err != nil {
		t.Fatal(err)
	}
	node := fakeNode{hashA: sendBlock(payer, payTo, quote.AmountRaw, quote.Representative)}
	srv := httptest.NewServer(node)
	defer srv.Close()

	input := paymentport.NanoVerifyInput{
		NodeURL: srv.URL, PayTo: payTo, XNOPerUnit: "2.5", OrderNo: "order-123", PayAmountCents: 1250, BlockHash: hashA,
	}
	hash, err := New(srv.Client()).VerifyPayment(context.Background(), input)
	if err != nil || hash != hashA {
		t.Fatalf("VerifyPayment = %q, %v", hash, err)
	}

	another := input
	another.OrderNo = "order-124"
	if _, err := New(nil).VerifyPayment(context.Background(), another); !errors.Is(err, ErrNotBound) {
		t.Fatalf("a block for order-123 paid order-124: %v", err)
	}
	dearer := input
	dearer.PayAmountCents = 1251
	if _, err := New(nil).VerifyPayment(context.Background(), dearer); !errors.Is(err, ErrWrongAmount) {
		t.Fatalf("a short payment was accepted: %v", err)
	}
	plain := input
	plain.NodeURL = "http://node.example.com"
	if _, err := New(nil).VerifyPayment(context.Background(), plain); err == nil {
		t.Fatal("a plain http node URL off loopback was used")
	}
}
