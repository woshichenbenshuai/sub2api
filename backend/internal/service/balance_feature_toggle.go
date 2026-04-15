package service

// Balance billing is intentionally disabled in this fork.
// Keep this as a centralized switch so all request-time checks and deductions
// stay consistent.
const balanceBillingEnabled = false

func isBalanceBillingEnabled() bool {
	return balanceBillingEnabled
}

// IsBalanceBillingEnabled reports whether user-balance billing is enabled.
func IsBalanceBillingEnabled() bool {
	return isBalanceBillingEnabled()
}
