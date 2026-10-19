//go:build !windows

package main

import "srdashboard/rwkbericht"

func readOptic(string) (rwkbericht.Bericht, error) {
	return rwkbericht.Bericht{}, ErrVoraussetzung
}
