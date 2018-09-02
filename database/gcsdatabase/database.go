/*
Sniperkit-Bot
- Status: analyzed
*/

// Sniperkit - 2018
// Status: Analyzed

package gcsdatabase

import (
	"cloud.google.com/go/datastore"
)

func New(client *datastore.Client) *Database {
	return &Database{
		Client: client,
	}
}

type Database struct {
	*datastore.Client
}
