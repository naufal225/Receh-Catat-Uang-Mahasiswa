package app

import (
	"receh/internal/modules"
	"receh/internal/modules/finance"
)

// Modules lists the feature modules this build includes, in start-up order.
// Removing a module = deleting its line here and its directory.
func Modules() []modules.Module {
	return []modules.Module{
		finance.New(),
	}
}
