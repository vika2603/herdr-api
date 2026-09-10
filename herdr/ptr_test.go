package herdr

import "testing"

func TestPtr(t *testing.T) {
	s := Ptr("value")
	if *s != "value" {
		t.Errorf("*Ptr(%q) = %q", "value", *s)
	}
	b := Ptr(false)
	if *b {
		t.Error("*Ptr(false) is true")
	}
	first, second := Ptr(1), Ptr(1)
	if first == second {
		t.Error("Ptr returned the same address for separate calls")
	}
}

func TestValue(t *testing.T) {
	if got := Value(Ptr(7)); got != 7 {
		t.Errorf("Value(Ptr(7)) = %d", got)
	}
	if got := Value[string](nil); got != "" {
		t.Errorf("Value[string](nil) = %q, want the zero value", got)
	}
	if got := Value[*PaneInfo](nil); got != nil {
		t.Errorf("Value[*PaneInfo](nil) = %v, want nil", got)
	}
}
