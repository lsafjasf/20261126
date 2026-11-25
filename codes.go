package cea608

// Miscellaneous control-command second bytes.
const (
	CmdRCL byte = 0x20 // Resume Caption Loading (pop-on)
	CmdBS  byte = 0x21 // Backspace
	CmdAOF byte = 0x22 // Alarm Off (reserved/older sets)
	CmdAON byte = 0x23 // Alarm On
	CmdDER byte = 0x24 // Delete to End of Row
	CmdRU2 byte = 0x25 // Roll-Up, two rows
	CmdRU3 byte = 0x26 // Roll-Up, three rows
	CmdRU4 byte = 0x27 // Roll-Up, four rows
	CmdFON byte = 0x28 // Flash On
	CmdRDC byte = 0x29 // Resume Direct Captioning (paint-on)
	CmdTR  byte = 0x2A // Text Restart
	CmdRTD byte = 0x2B // Resume Text Display
	CmdEDM byte = 0x2C // Erase Displayed Memory
	CmdCR  byte = 0x2D // Carriage Return
	CmdENM byte = 0x2E // Erase Non-Displayed Memory
	CmdEOC byte = 0x2F // End Of Caption / flip memories
)

var miscControlNames = map[byte]string{
	CmdRCL: "RCL",
	CmdBS:  "BS",
	CmdAOF: "AOF",
	CmdAON: "AON",
	CmdDER: "DER",
	CmdRU2: "RU2",
	CmdRU3: "RU3",
	CmdRU4: "RU4",
	CmdFON: "FON",
	CmdRDC: "RDC",
	CmdTR:  "TR",
	CmdRTD: "RTD",
	CmdEDM: "EDM",
	CmdCR:  "CR",
	CmdENM: "ENM",
	CmdEOC: "EOC",
}

func normalizeControlFirst(b1 byte) byte {
	if b1 >= 0x18 && b1 <= 0x1f {
		return b1 - 0x08
	}
	return b1
}

func isMiscControl(b1, b2 byte) bool {
	return normalizeControlFirst(b1) == 0x14 && b2 >= 0x20 && b2 <= 0x2f
}

func controlName(b1, b2 byte) string {
	if isMiscControl(b1, b2) {
		return miscControlNames[b2]
	}
	return ""
}

// pacRowIndent parses a Preamble Address Code. Returned row is zero-based.
func pacRowIndent(b1, b2 byte) (row, indent int, ok bool) {
	if b2 < 0x40 || b2 > 0x7f {
		return 0, 0, false
	}
	var base int
	switch b1 {
	case 0x10:
		base = 11
	case 0x11:
		base = 1
	case 0x12:
		base = 3
	case 0x13:
		base = 12
	case 0x14:
		base = 14
	case 0x15:
		base = 5
	case 0x16:
		base = 7
	case 0x17:
		base = 9
	default:
		return 0, 0, false
	}
	row = base
	if b1 != 0x10 && b2 >= 0x60 {
		row++
	}
	indent = int(b2&0x1e) >> 1 * 4
	if indent > 28 {
		indent = 0
	}
	return row - 1, indent, true
}
