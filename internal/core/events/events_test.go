package events

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEmitCallsListenersInOrder(t *testing.T) {
	b := New()
	var got []string
	b.On("user.created", func(_ context.Context, p any) error { got = append(got, "a:"+p.(string)); return nil })
	b.On("user.created", func(_ context.Context, p any) error { got = append(got, "b:"+p.(string)); return nil })

	if err := b.Emit(context.Background(), "user.created", "ada"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a:ada" || got[1] != "b:ada" {
		t.Errorf("listeners ran as %v, want [a:ada b:ada]", got)
	}
}

func TestEmitUnknownEventNoop(t *testing.T) {
	if err := New().Emit(context.Background(), "nobody.listening", nil); err != nil {
		t.Errorf("Emit with no listeners = %v, want nil", err)
	}
}

func TestEmitJoinsErrors(t *testing.T) {
	b := New()
	b.On("e", func(context.Context, any) error { return errors.New("one") })
	b.On("e", func(context.Context, any) error { return errors.New("two") })

	err := b.Emit(context.Background(), "e", nil)
	if err == nil || !strings.Contains(err.Error(), "one") || !strings.Contains(err.Error(), "two") {
		t.Errorf("Emit error = %v, want both listener errors joined", err)
	}
}

func TestPanicBecomesError(t *testing.T) {
	b := New()
	b.On("boom", func(context.Context, any) error { panic("kaboom") })
	if err := b.Emit(context.Background(), "boom", nil); err == nil {
		t.Error("a panicking listener should yield an error")
	}
}

func TestListenTypeSafe(t *testing.T) {
	type Created struct{ ID int }
	b := New()
	var gotID int
	Listen(b, "c", func(_ context.Context, p Created) error { gotID = p.ID; return nil })

	if err := b.Emit(context.Background(), "c", Created{ID: 7}); err != nil {
		t.Fatal(err)
	}
	if gotID != 7 {
		t.Errorf("typed listener got ID %d, want 7", gotID)
	}
	if err := b.Emit(context.Background(), "c", "not-a-Created"); err == nil {
		t.Error("a mismatched payload type should yield an error")
	}
}
