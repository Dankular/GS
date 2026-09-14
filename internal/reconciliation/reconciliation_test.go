package reconciliation

import "testing"

func TestFindingDifferenceIsProjectionMinusLedger(t *testing.T) {
	finding := Finding{WalletBalance: 17, LedgerBalance: 12}
	finding.Difference = finding.WalletBalance - finding.LedgerBalance
	if finding.Difference != 5 {
		t.Fatalf("difference = %d, want 5", finding.Difference)
	}
}
