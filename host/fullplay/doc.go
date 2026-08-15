// Package fullplay is the canonical regression suite for bundled games.
//
// Add a Scenario to Catalog when you ship a new plugin, a new player-count
// layout, or a display invariant that already broke once. Do not remove or
// rename existing IDs — TestCatalogIDsFrozen treats that as a regression.
//
//	go test ./host/fullplay
package fullplay
