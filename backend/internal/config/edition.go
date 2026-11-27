package config

// defaultBillingEnabled is the only line that differs between branches:
// true on b2c (the freemium product), false on personal (everything free).
// It lives alone in this file so merging b2c into personal never conflicts.
const defaultBillingEnabled = false
