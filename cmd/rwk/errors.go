package main

import "errors"

// ErrVoraussetzung is returned when Windows cannot render the PDF or create the German OCR engine.
// The slow DISM capability check is not run on this path.
var ErrVoraussetzung = errors.New("Deutsch-OCR fehlt oder die Windows-PDF-API ist nicht nutzbar. rwk.exe --check")
