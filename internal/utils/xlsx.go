package utils

import (
	"archive/zip"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// A minimal Excel (.xlsx) writer for report downloads: one sheet, a bold frozen
// header row with a filter, set column widths, text and number cells. Text
// cells stay text in Excel and LibreOffice — long IDs are not turned into
// 2.61E+019 and leading zeros are kept.

// Cell is one spreadsheet cell: text, or a number with a display format.
type Cell struct {
	Text   string
	Num    float64
	IsNum  bool
	Format NumFormat
}

// NumFormat is how a number cell is shown.
type NumFormat int

const (
	FmtGeneral  NumFormat = iota
	FmtInteger            // 1,234
	FmtDecimal2           // 1,234.50
)

// Text makes a text cell.
func Text(s string) Cell { return Cell{Text: s} }

// Int makes a whole-number cell (1,234).
func Int(n int64) Cell { return Cell{Num: float64(n), IsNum: true, Format: FmtInteger} }

// Dec2 makes a two-decimal number cell (1,234.50).
func Dec2(f float64) Cell { return Cell{Num: f, IsNum: true, Format: FmtDecimal2} }

// XLSXColumn is a column header and its width in characters.
type XLSXColumn struct {
	Header string
	Width  float64
}

// WriteXLSX writes a workbook with one sheet.
func WriteXLSX(w io.Writer, sheet string, cols []XLSXColumn, rows [][]Cell) error {
	z := zip.NewWriter(w)
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + xmlEsc(sheetName(sheet)) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		// Styles: 0 default, 1 bold header with a fill, 2 #,##0, 3 #,##0.00.
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FFE8EEF5"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/><xf numFmtId="3" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="4" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs></styleSheet>`},
	}
	for _, f := range files {
		fw, err := z.Create(f.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, f.body); err != nil {
			return err
		}
	}
	fw, err := z.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols>`)
	for i, c := range cols {
		fmt.Fprintf(&b, `<col min="%d" max="%d" width="%g" customWidth="1"/>`, i+1, i+1, c.Width)
	}
	b.WriteString(`</cols><sheetData><row r="1">`)
	for i, c := range cols {
		writeText(&b, colName(i)+"1", c.Header, 1)
	}
	b.WriteString(`</row>`)
	for r, row := range rows {
		n := strconv.Itoa(r + 2)
		b.WriteString(`<row r="` + n + `">`)
		for i, c := range row {
			ref := colName(i) + n
			if c.IsNum {
				style := map[NumFormat]int{FmtGeneral: 0, FmtInteger: 2, FmtDecimal2: 3}[c.Format]
				fmt.Fprintf(&b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, style, strconv.FormatFloat(c.Num, 'f', -1, 64))
			} else if c.Text != "" {
				writeText(&b, ref, c.Text, 0)
			}
		}
		b.WriteString(`</row>`)
		if b.Len() > 1<<20 { // stream large sheets
			if _, err := io.WriteString(fw, b.String()); err != nil {
				return err
			}
			b.Reset()
		}
	}
	b.WriteString(`</sheetData>`)
	if len(cols) > 0 {
		fmt.Fprintf(&b, `<autoFilter ref="A1:%s%d"/>`, colName(len(cols)-1), len(rows)+1)
	}
	b.WriteString(`</worksheet>`)
	if _, err := io.WriteString(fw, b.String()); err != nil {
		return err
	}
	return z.Close()
}

func writeText(b *strings.Builder, ref, s string, style int) {
	fmt.Fprintf(b, `<c r="%s" t="inlineStr"`, ref)
	if style != 0 {
		fmt.Fprintf(b, ` s="%d"`, style)
	}
	b.WriteString(`><is><t xml:space="preserve">` + xmlEsc(s) + `</t></is></c>`)
}

// colName is the spreadsheet column letter for index i (0 = A, 26 = AA).
func colName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// xmlEsc escapes text for XML and drops control characters XML cannot hold.
func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r < 0x20 && r != '\t' && r != '\n' && r != '\r':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sheetName makes a valid sheet name: at most 31 characters, none of []:*?/\.
func sheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, s)
	if len([]rune(s)) > 31 {
		s = string([]rune(s)[:31])
	}
	if s == "" {
		s = "Sheet1"
	}
	return s
}
