//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/zandercodes/gowinrt/winrt"

	"srdashboard/rwkbericht"
)

const (
	iidPdfStatics   = "433a0b5f-c007-4788-90f2-08143d922599"
	iidPdfPage      = "9db4b0c8-5320-4cfc-ad76-493fdad0e594"
	iidRenderOpts   = "3c98056f-b7cf-4c29-9a04-52d90267f425"
	iidFileStatics  = "5984c710-daf2-43c8-8bb4-a4d3eacfd03f"
	iidRandomStream = "905a0fe1-bc53-11df-8c49-001e4fc686da"
	iidDecStatics   = "438ccb26-bcef-4e95-bad6-23a822e58d01"
	iidFrameBitmap  = "fe287c9a-420c-4963-87ad-691436e08383"
	iidBmpStatics   = "df0385db-672f-4a9d-806e-c2442f343e86"
	iidLangFactory  = "9b0252ac-0c27-44f8-b792-9793fb66c63e"
	iidOcrStatics   = "5bffa85a-3384-3540-9940-699120d428a8"
	iidAsyncInfo    = "00000036-0000-0000-c000-000000000046"
)

func readOptic(path string) (rwkbericht.Bericht, error) {
	words, width, err := recognize(path)
	if err != nil {
		return rwkbericht.Bericht{}, err
	}
	bericht := ParseSheet(words, width)
	if !hasShooters(bericht) {
		return rwkbericht.Bericht{}, fmt.Errorf("keine Schützen im OpticScore-PDF erkannt")
	}
	return bericht, nil
}

func recognize(path string) ([]Word, float64, error) {
	if err := ensureWinRT(); err != nil {
		return nil, 0, ErrVoraussetzung
	}
	pdfFactory, err := factory("Windows.Data.Pdf.PdfDocument", iidPdfStatics)
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer pdfFactory.Release()
	engine, err := germanOCR()
	if err != nil || engine == nil {
		return nil, 0, ErrVoraussetzung
	}
	defer engine.Release()

	fileStatics, err := factory("Windows.Storage.StorageFile", iidFileStatics)
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer fileStatics.Release()
	pathHS, err := winrt.NewHString(path)
	if err != nil {
		return nil, 0, err
	}
	defer winrt.DeleteHString(pathHS)
	var op *winrt.IUnknown
	if _, err := call(fileStatics, 6, uintptr(pathHS), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, fmt.Errorf("PDF konnte nicht geöffnet werden: %w", err)
	}
	file, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, fmt.Errorf("PDF konnte nicht geöffnet werden: %w", err)
	}
	defer file.Release()

	op = nil
	if _, err := call(pdfFactory, 6, uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, fmt.Errorf("PDF konnte nicht gelesen werden: %w", err)
	}
	doc, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, fmt.Errorf("PDF konnte nicht gelesen werden: %w", err)
	}
	defer doc.Release()

	var page *winrt.IUnknown
	if _, err := call(doc, 6, uintptr(uint32(0)), uintptr(unsafe.Pointer(&page))); err != nil {
		return nil, 0, err
	}
	defer page.Release()

	streamObj, err := winrt.RoActivateInstance("Windows.Storage.Streams.InMemoryRandomAccessStream")
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer streamObj.Release()
	stream, err := qi(&streamObj.IUnknown, iidRandomStream)
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer stream.Release()

	optObj, err := winrt.RoActivateInstance("Windows.Data.Pdf.PdfPageRenderOptions")
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer optObj.Release()
	opts, err := qi(&optObj.IUnknown, iidRenderOpts)
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer opts.Release()
	if _, err := call(opts, 9, uintptr(uint32(1700))); err != nil {
		return nil, 0, err
	}
	_ = iidPdfPage

	op = nil
	if _, err := call(page, 7, uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(opts)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, fmt.Errorf("PDF-Seite konnte nicht gezeichnet werden: %w", err)
	}
	if err := asyncAction(op); err != nil {
		release(op)
		return nil, 0, fmt.Errorf("PDF-Seite konnte nicht gezeichnet werden: %w", err)
	}
	release(op)
	if _, err := call(stream, 11, uintptr(uint64(0))); err != nil {
		return nil, 0, err
	}

	decStatics, err := factory("Windows.Graphics.Imaging.BitmapDecoder", iidDecStatics)
	if err != nil {
		return nil, 0, ErrVoraussetzung
	}
	defer decStatics.Release()
	op = nil
	if _, err := call(decStatics, 14, uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, fmt.Errorf("gerenderte Seite konnte nicht gelesen werden: %w", err)
	}
	decoder, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, fmt.Errorf("gerenderte Seite konnte nicht gelesen werden: %w", err)
	}
	defer decoder.Release()

	op = nil
	if _, err := call(decoder, 10, uintptr(uint32(0)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, err
	}
	frame, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, err
	}
	defer frame.Release()
	soft, err := qi(frame, iidFrameBitmap)
	if err != nil {
		return nil, 0, err
	}
	defer soft.Release()
	op = nil
	if _, err := call(soft, 6, uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, err
	}
	bitmap, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, err
	}
	defer bitmap.Release()
	if converted, convErr := toBgra(bitmap); convErr == nil && converted != nil {
		defer converted.Release()
		bitmap = converted
	}

	op = nil
	if _, err := call(engine, 6, uintptr(unsafe.Pointer(bitmap)), uintptr(unsafe.Pointer(&op))); err != nil {
		return nil, 0, fmt.Errorf("OCR fehlgeschlagen: %w", err)
	}
	result, err := asyncResult(op)
	release(op)
	if err != nil {
		return nil, 0, fmt.Errorf("OCR fehlgeschlagen: %w", err)
	}
	defer result.Release()

	width, _ := readU32(bitmap, 8)
	words, err := ocrWords(result)
	if err != nil {
		return nil, 0, fmt.Errorf("OCR fehlgeschlagen: %w", err)
	}
	return words, float64(width), nil
}

func germanOCR() (*winrt.IUnknown, error) {
	langFactory, err := factory("Windows.Globalization.Language", iidLangFactory)
	if err != nil {
		return nil, err
	}
	defer langFactory.Release()
	tag, err := winrt.NewHString("de-DE")
	if err != nil {
		return nil, err
	}
	defer winrt.DeleteHString(tag)
	var lang *winrt.IUnknown
	if _, err := call(langFactory, 6, uintptr(tag), uintptr(unsafe.Pointer(&lang))); err != nil {
		return nil, err
	}
	defer lang.Release()
	ocrStatics, err := factory("Windows.Media.Ocr.OcrEngine", iidOcrStatics)
	if err != nil {
		return nil, err
	}
	defer ocrStatics.Release()
	var engine *winrt.IUnknown
	if _, err := call(ocrStatics, 9, uintptr(unsafe.Pointer(lang)), uintptr(unsafe.Pointer(&engine))); err != nil || engine == nil {
		return nil, ErrVoraussetzung
	}
	return engine, nil
}

func toBgra(bitmap *winrt.IUnknown) (*winrt.IUnknown, error) {
	statics, err := factory("Windows.Graphics.Imaging.SoftwareBitmap", iidBmpStatics)
	if err != nil {
		return nil, err
	}
	defer statics.Release()
	var out *winrt.IUnknown
	// Convert(bitmap, Bgra8=87, Premultiplied=0)
	if _, err := call(statics, 8, uintptr(unsafe.Pointer(bitmap)), uintptr(uint32(87)), uintptr(uint32(0)), uintptr(unsafe.Pointer(&out))); err != nil {
		return nil, err
	}
	return out, nil
}

type abiRect struct {
	X, Y, W, H float32
}

func ocrWords(result *winrt.IUnknown) ([]Word, error) {
	var lines *winrt.IUnknown
	if _, err := call(result, 6, uintptr(unsafe.Pointer(&lines))); err != nil {
		return nil, err
	}
	defer lines.Release()
	n, err := readU32(lines, 7)
	if err != nil {
		return nil, err
	}
	var words []Word
	for i := uint32(0); i < n; i++ {
		var line *winrt.IUnknown
		if _, err := call(lines, 6, uintptr(i), uintptr(unsafe.Pointer(&line))); err != nil {
			return nil, err
		}
		lineWords, err := wordsOnLine(line)
		line.Release()
		if err != nil {
			return nil, err
		}
		words = append(words, lineWords...)
	}
	return words, nil
}

func wordsOnLine(line *winrt.IUnknown) ([]Word, error) {
	var view *winrt.IUnknown
	if _, err := call(line, 6, uintptr(unsafe.Pointer(&view))); err != nil {
		return nil, err
	}
	defer view.Release()
	n, err := readU32(view, 7)
	if err != nil {
		return nil, err
	}
	out := make([]Word, 0, n)
	for i := uint32(0); i < n; i++ {
		var word *winrt.IUnknown
		if _, err := call(view, 6, uintptr(i), uintptr(unsafe.Pointer(&word))); err != nil {
			return nil, err
		}
		var rc abiRect
		_, rectErr := call(word, 6, uintptr(unsafe.Pointer(&rc)))
		text, textErr := readString(word, 7)
		word.Release()
		if rectErr != nil {
			return nil, rectErr
		}
		if textErr != nil {
			return nil, textErr
		}
		if text == "" {
			continue
		}
		out = append(out, Word{Text: text, X: float64(rc.X), Y: float64(rc.Y), W: float64(rc.W), H: float64(rc.H)})
	}
	return out, nil
}

func ensureWinRT() error {
	err := winrt.RoInitialize(1)
	if err == nil {
		return nil
	}
	he, ok := err.(*winrt.HResultError)
	if ok && (he.Code() == 1 || he.Code() == 0x80010106) {
		return nil
	}
	return err
}

func factory(class, iid string) (*winrt.IUnknown, error) {
	ins, err := winrt.RoGetActivationFactory(class, winrt.NewGUID(iid))
	if err != nil || ins == nil {
		return nil, ErrVoraussetzung
	}
	return &ins.IUnknown, nil
}

func qi(obj *winrt.IUnknown, iid string) (*winrt.IUnknown, error) {
	return obj.QueryInterface(winrt.NewGUID(iid))
}

func release(obj *winrt.IUnknown) {
	if obj != nil {
		obj.Release()
	}
}

func call(obj *winrt.IUnknown, slot int, args ...uintptr) (uintptr, error) {
	if obj == nil || obj.RawVTable == nil {
		return 0, ErrVoraussetzung
	}
	vt := unsafe.Slice((*uintptr)(unsafe.Pointer(obj.RawVTable)), slot+1)
	all := make([]uintptr, 1, 1+len(args))
	all[0] = uintptr(unsafe.Pointer(obj))
	all = append(all, args...)
	hr, _, _ := syscall.SyscallN(vt[slot], all...)
	if hr != 0 {
		return hr, winrt.NewError(hr)
	}
	return hr, nil
}

func asyncResult(op *winrt.IUnknown) (*winrt.IUnknown, error) {
	if err := waitAsync(op); err != nil {
		return nil, err
	}
	var out *winrt.IUnknown
	if _, err := call(op, 8, uintptr(unsafe.Pointer(&out))); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("leeres Ergebnis")
	}
	return out, nil
}

func asyncAction(op *winrt.IUnknown) error {
	if err := waitAsync(op); err != nil {
		return err
	}
	_, err := call(op, 8)
	return err
}

func waitAsync(op *winrt.IUnknown) error {
	if op == nil {
		return fmt.Errorf("kein Async-Vorgang")
	}
	info, err := qi(op, iidAsyncInfo)
	if err != nil {
		return err
	}
	defer info.Release()
	deadline := time.Now().Add(45 * time.Second)
	for {
		status, err := readU32(info, 7)
		if err != nil {
			return err
		}
		switch status {
		case 1:
			return nil
		case 2, 3:
			return fmt.Errorf("async status %d", status)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("zeitüberschreitung")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readU32(obj *winrt.IUnknown, slot int) (uint32, error) {
	var n uint32
	_, err := call(obj, slot, uintptr(unsafe.Pointer(&n)))
	return n, err
}

func readString(obj *winrt.IUnknown, slot int) (string, error) {
	var hs winrt.HString
	if _, err := call(obj, slot, uintptr(unsafe.Pointer(&hs))); err != nil {
		return "", err
	}
	defer winrt.DeleteHString(hs)
	return hs.String(), nil
}
