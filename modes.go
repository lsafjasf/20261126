package cea608

type modeState interface {
	mode() Mode
	putChar(d *Decoder, ch rune)
	pac(d *Decoder, row, indent int)
	carriageReturn(d *Decoder)
	endOfCaption(d *Decoder)
}

func writeAt(d *Decoder, target *screen, ch rune) {
	if d.Caption.Col >= 0 && d.Caption.Col < Cols {
		target.put(d.Caption.Row, d.Caption.Col, ch)
	}
	d.Caption.Col++
}

type noneState struct{}

func (noneState) mode() Mode { return ModeNone }
func (noneState) putChar(d *Decoder, ch rune) {
	// Tolerant recovery: retain plain text even before a mode command.
	writeAt(d, &d.displayed, ch)
}
func (noneState) pac(d *Decoder, row, indent int) {
	d.Caption.Row, d.Caption.Col = row, indent
}
func (noneState) carriageReturn(*Decoder) {}
func (noneState) endOfCaption(*Decoder)   {}

type popOnState struct{}

func (popOnState) mode() Mode                  { return ModePopOn }
func (popOnState) putChar(d *Decoder, ch rune) { writeAt(d, &d.stored, ch) }
func (popOnState) pac(d *Decoder, row, indent int) {
	d.Caption.Row, d.Caption.Col = row, indent
}
func (popOnState) carriageReturn(*Decoder) {}
func (popOnState) endOfCaption(d *Decoder) {
	d.displayed, d.stored = d.stored, d.displayed
	d.appendCueFromDisplay()
	d.Caption.Row, d.Caption.Col = 0, 0
}

type paintOnState struct{}

func (paintOnState) mode() Mode                  { return ModePaintOn }
func (paintOnState) putChar(d *Decoder, ch rune) { writeAt(d, &d.displayed, ch) }
func (paintOnState) pac(d *Decoder, row, indent int) {
	d.Caption.Row, d.Caption.Col = row, indent
}
func (paintOnState) carriageReturn(*Decoder) {}
func (paintOnState) endOfCaption(d *Decoder) {
	d.appendCueFromDisplay()
	d.displayed, d.stored = d.stored, d.displayed
	d.enterMode(ModePopOn)
}

type rollUpState struct{}

func (rollUpState) mode() Mode { return ModeRollUp }
func (rollUpState) putChar(d *Decoder, ch rune) {
	d.Caption.Row = d.Caption.BaseRow
	writeAt(d, &d.displayed, ch)
}
func (rollUpState) pac(d *Decoder, row, indent int) {
	d.Caption.BaseRow = row
	d.Caption.Row, d.Caption.Col = row, indent
}
func (rollUpState) carriageReturn(d *Decoder) {
	top := d.Caption.BaseRow - d.Caption.RollRows + 1
	d.displayed.scrollUp(top, d.Caption.BaseRow)
	d.Caption.Row, d.Caption.Col = d.Caption.BaseRow, 0
}
func (rollUpState) endOfCaption(*Decoder) {}

type textState struct{}

func (textState) mode() Mode { return ModeText }
func (textState) putChar(d *Decoder, ch rune) {
	writeAt(d, &d.displayed, ch)
}
func (textState) pac(d *Decoder, row, indent int) {
	d.Caption.Row, d.Caption.Col = row, indent
}
func (textState) carriageReturn(d *Decoder) {
	d.displayed.scrollUp(0, Rows-1)
	d.Caption.Row = Rows - 1
	d.Caption.Col = 0
}
func (textState) endOfCaption(d *Decoder) { d.appendCueFromDisplay() }
