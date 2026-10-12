// internal/features/documentprocessing/textreading/errors.go
package textreading

import "errors"

var ErrUnreadablePDF = errors.New("PDF has no usable selectable text")
