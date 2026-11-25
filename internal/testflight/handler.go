// Package testflight provides movement and time controls for terrain testing.
package testflight

import "github.com/df-mc/dragonfly/server/player"

// Handler owns flight settings on the player's world transaction and discards them on quit.
type Handler struct {
	player.NopHandler
	flight flySpeedState
}

// FlightState exposes flight settings to the input wrapper. Promoting this
// method through a composite handler keeps flight working when other gameplay
// handlers are installed on the same player.
func (h *Handler) FlightState() *flySpeedState { return &h.flight }
