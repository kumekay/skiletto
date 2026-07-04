package adapter

import (
	"errors"
	"strings"
	"testing"
)

// TestRunLinkChainFallsBack verifies the fallback chain tries strategies in
// order, stops at the first success, and reports which strategy won. This is
// the platform-independent decision logic; the Windows chain (symlink →
// junction → copy) is exercised in CI, but the ordering and short-circuit
// behavior are testable anywhere by injecting a failing first step.
func TestRunLinkChainFallsBack(t *testing.T) {
	var attempted []LinkStrategy
	steps := []linkStep{
		{StrategySymlink, func(string, string) error {
			attempted = append(attempted, StrategySymlink)
			return errors.New("symlink privilege denied")
		}},
		{StrategyJunction, func(string, string) error {
			attempted = append(attempted, StrategyJunction)
			return nil
		}},
		{StrategyCopy, func(string, string) error {
			attempted = append(attempted, StrategyCopy)
			return errors.New("should not reach copy")
		}},
	}
	got, err := runLinkChain("link", "target", steps)
	if err != nil {
		t.Fatalf("runLinkChain: %v", err)
	}
	if got != StrategyJunction {
		t.Errorf("strategy = %q, want %q", got, StrategyJunction)
	}
	if len(attempted) != 2 || attempted[0] != StrategySymlink || attempted[1] != StrategyJunction {
		t.Errorf("attempted = %v, want [symlink junction] and no copy", attempted)
	}
}

// TestRunLinkChainAllFail surfaces every underlying cause when no strategy
// succeeds, so a Windows failure is diagnosable.
func TestRunLinkChainAllFail(t *testing.T) {
	steps := []linkStep{
		{StrategySymlink, func(string, string) error { return errors.New("cause-symlink") }},
		{StrategyJunction, func(string, string) error { return errors.New("cause-junction") }},
	}
	_, err := runLinkChain("link", "target", steps)
	if err == nil {
		t.Fatal("want error when all strategies fail")
	}
	if !strings.Contains(err.Error(), "cause-symlink") || !strings.Contains(err.Error(), "cause-junction") {
		t.Errorf("err = %v, want both causes surfaced", err)
	}
}

// TestRunLinkChainSingleStepVerbatim keeps the unix path (symlink only)
// byte-for-byte: a lone step's error is returned unwrapped, exactly as a
// direct os.Symlink would.
func TestRunLinkChainSingleStepVerbatim(t *testing.T) {
	sentinel := errors.New("sentinel")
	steps := []linkStep{{StrategySymlink, func(string, string) error { return sentinel }}}
	_, err := runLinkChain("link", "target", steps)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the verbatim sentinel error", err)
	}
}
