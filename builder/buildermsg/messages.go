/*
Sniperkit-Bot
- Status: analyzed
*/

// Sniperkit - 2018
// Status: Analyzed

package buildermsg

import (
	"encoding/gob"
)

func RegisterTypes() {
	gob.Register(Building{})
}

type Building struct {
	Starting bool
	Message  string
	Done     bool
}
