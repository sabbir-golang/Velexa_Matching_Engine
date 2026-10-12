package main

// instref.go — fills in the classification an instrument master leaves blank
// (asset class, share category, sector, board) so the simulator's market looks
// like the real DSE. Real tickers take their values from the DSE reference file
// (instref_data.go); the synthetic ones get a deterministic, DSE-weighted pick
// seeded by the symbol, so the same scrip always gets the same answer.

import (
	"hash/fnv"
	"strings"
)

// weighted is a value with a selection weight.
type weighted struct {
	v string
	w int
}

// DSE equity mix, from the reference file (374 equities).
var (
	catWeights = []weighted{{"A", 57}, {"B", 29}, {"Z", 8}, {"S", 5}, {"N", 1}}
	secWeights = []weighted{{"Textile", 59}, {"Insurance", 57}, {"Engineering", 45}, {"PharmaChem", 38},
		{"Bank", 35}, {"FoodAllied", 24}, {"FuelPower", 23}, {"Financial In", 23}, {"Misc", 20}, {"IT", 11},
		{"Tannery", 7}, {"Cement", 7}, {"PaperPrint", 6}, {"Ceramic", 5}, {"TravelLeisur", 4},
		{"ServRealEst", 4}, {"Telecom", 3}, {"Jute", 3}}
)

// sectorHints map words in a symbol or company name to a sector, so a synthetic
// "BANK042" lands in Bank rather than a random one.
var sectorHints = []struct{ word, sector string }{
	{"BANK", "Bank"}, {"INSUR", "Insurance"}, {"LIFE", "Insurance"}, {"PHARMA", "PharmaChem"},
	{"CHEM", "PharmaChem"}, {"TEX", "Textile"}, {"SPIN", "Textile"}, {"KNIT", "Textile"}, {"DENIM", "Textile"},
	{"CEMENT", "Cement"}, {"POWER", "FuelPower"}, {"ENERGY", "FuelPower"}, {"GAS", "FuelPower"},
	{"OIL", "FuelPower"}, {"SOFT", "IT"}, {"TECH", "IT"}, {"FOOD", "FoodAllied"}, {"SUGAR", "FoodAllied"},
	{"CERAM", "Ceramic"}, {"JUTE", "Jute"}, {"LEATHER", "Tannery"}, {"PAPER", "PaperPrint"},
	{"STEEL", "Engineering"}, {"CABLE", "Engineering"}, {"ENG", "Engineering"}, {"LEASING", "Financial In"},
	{"FINANCE", "Financial In"}, {"INVEST", "Financial In"}, {"TELE", "Telecom"}, {"HOTEL", "TravelLeisur"},
	{"TRAVEL", "TravelLeisur"}, {"REAL", "ServRealEst"}, {"PROPERT", "ServRealEst"},
}

func hashOf(sym, salt string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(salt + sym))
	return h.Sum32()
}

func pickWeighted(list []weighted, h uint32) string {
	total := 0
	for _, x := range list {
		total += x.w
	}
	n := int(h % uint32(total))
	for _, x := range list {
		if n < x.w {
			return x.v
		}
		n -= x.w
	}
	return list[0].v
}

// boardFor is the board label an instrument of this class/category trades on.
func boardFor(assetClass, category string) string {
	switch assetClass {
	case "MF":
		return "Mutual Fund"
	case "CB", "DB":
		return "Bond"
	}
	switch category {
	case "S":
		return "SME / ATB"
	case "B", "N", "Z":
		return "Main (" + category + ")"
	}
	return "Main (A)"
}

// sectorForClass is the fixed sector of the non-equity classes.
func sectorForClass(assetClass string) string {
	switch assetClass {
	case "MF":
		return "MutFund"
	case "CB":
		return "CorpBond"
	case "DB":
		return "Debenture"
	}
	return ""
}

// enrichMeta fills what a master leaves blank. It only touches an instrument that
// has never been classified (no asset class and no sector). A real DSE ticker
// takes its reference values; any other scrip is synthesised only when its board
// is the legacy placeholder ("" or "MAIN"), so a real master is never overwritten
// with invented data.
func enrichMeta(sym, name string, m instrMetaT) instrMetaT {
	if m.AssetClass != "" || m.Sector != "" {
		return m
	}
	legacy := m.Board == "" || strings.EqualFold(m.Board, "MAIN")
	var class, category, sector string
	if r, ok := dseRef[sym]; ok {
		class, category, sector = r.AssetClass, r.Category, r.Sector
		if m.ISIN == "" {
			m.ISIN = r.ISIN
		}
	} else if legacy {
		up := strings.ToUpper(sym + " " + name)
		class = "EQ"
		if strings.Contains(up, "FUND") || strings.HasSuffix(strings.ToUpper(sym), "MF") {
			class = "MF"
		}
		if class == "EQ" {
			category = pickWeighted(catWeights, hashOf(sym, "cat"))
			sector = pickWeighted(secWeights, hashOf(sym, "sec"))
			for _, h := range sectorHints {
				if strings.Contains(up, h.word) {
					sector = h.sector
					break
				}
			}
		} else {
			category = "A"
		}
	} else {
		return m
	}
	m.AssetClass = class
	if m.Sector == "" {
		m.Sector = sector
	}
	if m.Sector == "" {
		m.Sector = sectorForClass(class)
	}
	if legacy {
		m.Board = boardFor(class, category)
	}
	return m
}

// enrich applies enrichMeta to every instrument of a parsed master.
func (u universeSet) enrich() {
	for _, in := range u.Instr {
		if m, ok := u.Meta[in.Symbol]; ok {
			u.Meta[in.Symbol] = enrichMeta(in.Symbol, in.Name, m)
		}
	}
}

// sectorLabels are readable names for the DSE sector codes.
var sectorLabels = map[string]string{
	"Bank": "Bank", "Cement": "Cement", "Ceramic": "Ceramic", "CorpBond": "Corporate Bond",
	"Debenture": "Debenture", "Engineering": "Engineering", "Financial In": "Financial Institutions",
	"FoodAllied": "Food & Allied", "FuelPower": "Fuel & Power", "Insurance": "Insurance", "IT": "IT",
	"Jute": "Jute", "Misc": "Miscellaneous", "MutFund": "Mutual Fund", "PaperPrint": "Paper & Printing",
	"PharmaChem": "Pharmaceuticals & Chemicals", "ServRealEst": "Services & Real Estate",
	"Tannery": "Tannery", "Telecom": "Telecom", "Textile": "Textile", "TravelLeisur": "Travel & Leisure",
}

// assetClassLabel is the readable name of an asset-class code ("EQ" -> "Equity").
func assetClassLabel(code string) string {
	for _, a := range assetClasses {
		if a.Code == code {
			return a.Label
		}
	}
	return code
}
