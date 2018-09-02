/*
Sniperkit-Bot
- Status: analyzed
*/

// Sniperkit - 2018
// Status: Analyzed

package gettermsg

import (
	"encoding/gob"
)

func RegisterTypes() {
	gob.Register(Downloading{})
}

type Downloading struct {
	Starting bool
	Message  string
	Done     bool
}
