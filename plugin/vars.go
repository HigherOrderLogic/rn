package plugin

import "sync"

var (
	// Used to cache proto clients by handlerID
	// These maps are never cleaned up from the plugin's memory
	// but each plugin should have at least 1 browser and editor
	// per permission per process so in practice it's not a problem
	clients sync.Map
)
