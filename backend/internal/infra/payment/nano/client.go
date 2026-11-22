package nano

import (
	"context"
	"net/http"
	"strings"

	paymentport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/payment"
)

// Client maps the payment port to the Nano protocol. It only quotes and
// reads; it never signs, sends or holds a key.
type Client struct {
	httpClient *http.Client
}

// New creates the Nano adapter. httpClient may be nil, in which case a
// client with a 10 second timeout is used for node reads.
func New(httpClient *http.Client) *Client {
	return &Client{httpClient: httpClient}
}

func (c *Client) expectation(payTo, xnoPerUnit, orderNo string, amountCents int64) (Expectation, error) {
	payTo = strings.TrimSpace(payTo)
	if err := ValidateAccount(payTo); err != nil {
		return Expectation{}, err
	}
	if strings.TrimSpace(orderNo) == "" {
		return Expectation{}, ErrInvalidAmount
	}
	rate, err := ParseRate(xnoPerUnit)
	if err != nil {
		return Expectation{}, err
	}
	raw, err := rate.Raw(amountCents)
	if err != nil {
		return Expectation{}, err
	}
	return Expectation{OrderNo: orderNo, PayTo: payTo, Raw: raw}, nil
}

// Quote returns the one payment the order accepts.
func (c *Client) Quote(_ context.Context, input paymentport.NanoCheckoutInput) (paymentport.NanoCheckout, error) {
	want, err := c.expectation(input.PayTo, input.XNOPerUnit, input.OrderNo, input.PayAmountCents)
	if err != nil {
		return paymentport.NanoCheckout{}, err
	}
	raw := want.Raw.String()
	representative := CommitmentAccount(want.OrderNo, want.PayTo, raw)
	return paymentport.NanoCheckout{
		Network:        Network,
		Scheme:         Scheme,
		PayTo:          want.PayTo,
		AmountRaw:      raw,
		Representative: representative,
		PaymentURI:     PaymentURI(want.PayTo, raw, representative),
	}, nil
}

// VerifyPayment reads the block from the operator's node and proves it pays
// the order exactly. It returns the normalized block hash, which the caller
// stores as the order's external payment id.
func (c *Client) VerifyPayment(ctx context.Context, input paymentport.NanoVerifyInput) (string, error) {
	want, err := c.expectation(input.PayTo, input.XNOPerUnit, input.OrderNo, input.PayAmountCents)
	if err != nil {
		return "", err
	}
	if err := ValidateNodeURL(input.NodeURL); err != nil {
		return "", err
	}
	node := &Node{URL: input.NodeURL}
	if c != nil {
		node.HTTPClient = c.httpClient
	}
	block, err := node.BlockInfo(ctx, input.BlockHash)
	if err != nil {
		return "", err
	}
	if err := Verify(block, want); err != nil {
		return "", err
	}
	return block.Hash, nil
}
