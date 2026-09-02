package main

import (
	"sync/atomic"
	"strings"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

// instrument universe — 400 DSE tickers (real blue-chips + sector universe).
var universe = []Instrument{
	{"GP", "Grameenphone Ltd.", 331.20}, {"SQURPHARMA", "Square Pharmaceuticals", 214.50},
	{"BEXIMCO", "Beximco Ltd.", 118.90}, {"ROBI", "Robi Axiata Ltd.", 29.40},
	{"BATBC", "British American Tobacco BD", 512.00}, {"WALTONHIL", "Walton Hi-Tech", 780.30},
	{"RENATA", "Renata Ltd.", 1245.00}, {"LHBL", "LafargeHolcim BD", 66.80},
	{"BRACBANK", "BRAC Bank Ltd.", 44.20}, {"CITYBANK", "City Bank Ltd.", 22.10},
	{"SSSTEEL", "SS Steel Ltd.", 8.90}, {"GEMINISEA", "Gemini Sea Food", 796.40},
	{"ORIONPHARM", "Orion Pharma Ltd.", 58.70}, {"BSCCL", "BD Submarine Cable", 142.30},
	{"IFIC", "IFIC Bank Ltd.", 11.60}, {"UPGDCL", "United Power Generation", 158.00},
	{"ABBANK", "Abbank Ltd.", 387.26}, {"ALARABANK", "Alarabank Ltd.", 24.76},
	{"BANKASIA", "Bankasia Ltd.", 172.27}, {"DHAKABANK", "Dhakabank Ltd.", 141.69},
	{"DUTCHBANGL", "Dutchbangl Ltd.", 444.52}, {"EBL", "Ebl Ltd.", 409.25},
	{"EXIMBANK", "Eximbank Ltd.", 536.39}, {"FIRSTSBANK", "Firstsbank Ltd.", 61.29},
	{"ISLAMIBANK", "Islamibank Ltd.", 258.93}, {"JAMUNABANK", "Jamunabank Ltd.", 27.58},
	{"MERCANBANK", "Mercanbank Ltd.", 139.00}, {"MTB", "Mtb Ltd.", 308.16},
	{"NBL", "Nbl Ltd.", 25.66}, {"NCCBANK", "Nccbank Ltd.", 127.31},
	{"ONEBANKPLC", "Onebankplc Ltd.", 393.43}, {"PREMIERBAN", "Premierban Ltd.", 331.52},
	{"PRIMEBANK", "Primebank Ltd.", 140.06}, {"PUBALIBANK", "Pubalibank Ltd.", 357.67},
	{"RUPALIBANK", "Rupalibank Ltd.", 487.56}, {"SOUTHEASTB", "Southeastb Ltd.", 13.83},
	{"SHAHJABANK", "Shahjabank Ltd.", 485.43}, {"SIBL", "Sibl Ltd.", 421.90},
	{"STANDBANKL", "Standbankl Ltd.", 210.75}, {"TRUSTBANK", "Trustbank Ltd.", 101.73},
	{"UCB", "Ucb Ltd.", 574.76}, {"UTTARABANK", "Uttarabank Ltd.", 208.59},
	{"ICBIBANK", "Icbibank Ltd.", 64.72}, {"BENGALCOM", "Bengalcom Ltd.", 67.06},
	{"ACI", "Aci Ltd.", 510.02}, {"ACMELAB", "Acmelab Ltd.", 366.20},
	{"IBNSINA", "Ibnsina Ltd.", 486.21}, {"KOHINOOR", "Kohinoor Ltd.", 440.54},
	{"PHARMAID", "Pharmaid Ltd.", 326.37}, {"SILVAPHL", "Silvaphl Ltd.", 584.14},
	{"AMBEEPHA", "Ambeepha Ltd.", 233.34}, {"CENTRALPHL", "Centralphl Ltd.", 335.70},
	{"JMISMDL", "Jmismdl Ltd.", 499.35}, {"LIBRAINFU", "Librainfu Ltd.", 374.93},
	{"ORIONINFU", "Orioninfu Ltd.", 518.41}, {"ACIFORMULA", "Aciformula Ltd.", 350.64},
	{"BXPHARMA", "Bxpharma Ltd.", 425.70}, {"GLOBALHEAV", "Globalheav Ltd.", 37.04},
	{"INDOPHARMA", "Indopharma Ltd.", 144.46}, {"MARICO", "Marico Ltd.", 180.74},
	{"NAVANAPHAR", "Navanaphar Ltd.", 57.08}, {"FARCHEM", "Farchem Ltd.", 147.35},
	{"ACTIVEFINE", "Activefine Ltd.", 69.59}, {"SALVOCHEM", "Salvochem Ltd.", 174.00},
	{"PADMAOIL", "Padmaoil Ltd.", 385.05}, {"MEGHNAPET", "Meghnapet Ltd.", 225.25},
	{"JAMUNAOIL", "Jamunaoil Ltd.", 228.41}, {"MPETROLEUM", "Mpetroleum Ltd.", 133.61},
	{"TITASGAS", "Titasgas Ltd.", 167.52}, {"DESCO", "Desco Ltd.", 562.63},
	{"POWERGRID", "Powergrid Ltd.", 392.34}, {"SUMITPOWER", "Sumitpower Ltd.", 369.39},
	{"DOREENPWR", "Doreenpwr Ltd.", 110.97}, {"BARKAPOWER", "Barkapower Ltd.", 440.18},
	{"KPCL", "Kpcl Ltd.", 106.41}, {"UNIQUEHRL", "Uniquehrl Ltd.", 233.88},
	{"ACPL", "Acpl Ltd.", 593.82}, {"GBBPOWER", "Gbbpower Ltd.", 387.60},
	{"SPCL", "Spcl Ltd.", 338.60}, {"EGEN", "Egen Ltd.", 413.92},
	{"WZPOWERGEN", "Wzpowergen Ltd.", 507.28}, {"LINDEBD", "Lindebd Ltd.", 467.84},
	{"INTRACO", "Intraco Ltd.", 145.14}, {"CVOPRL", "Cvoprl Ltd.", 28.94},
	{"SQUARETEXT", "Squaretext Ltd.", 196.12}, {"MATINSPINN", "Matinspinn Ltd.", 167.97},
	{"ARGONDENIM", "Argondenim Ltd.", 134.48}, {"ENVOYTEX", "Envoytex Ltd.", 566.32},
	{"PARAMOUNT", "Paramount Ltd.", 527.06}, {"HRTEX", "Hrtex Ltd.", 195.66},
	{"DELTASPINN", "Deltaspinn Ltd.", 396.71}, {"SAIHAMTEX", "Saihamtex Ltd.", 243.42},
	{"GENNEXT", "Gennext Ltd.", 549.58}, {"TALLUSPIN", "Talluspin Ltd.", 280.72},
	{"MOZAFFAR", "Mozaffar Ltd.", 166.28}, {"SIMTEX", "Simtex Ltd.", 155.51},
	{"DACCADYE", "Daccadye Ltd.", 341.21}, {"ALLTEX", "Alltex Ltd.", 165.02},
	{"ZAHINTEX", "Zahintex Ltd.", 354.91}, {"FAMILYTEX", "Familytex Ltd.", 539.72},
	{"NURANI", "Nurani Ltd.", 245.65}, {"RINGSHINE", "Ringshine Ltd.", 139.40},
	{"DRAGONSWR", "Dragonswr Ltd.", 598.55}, {"SHASHADNIT", "Shashadnit Ltd.", 310.62},
	{"NEWLINE", "Newline Ltd.", 63.64}, {"MLDYEING", "Mldyeing Ltd.", 37.80},
	{"SAFKOSPINN", "Safkospinn Ltd.", 74.69}, {"APEXSPINN", "Apexspinn Ltd.", 380.19},
	{"MITHUNKNIT", "Mithunknit Ltd.", 477.33}, {"ESQUIRENIT", "Esquirenit Ltd.", 259.07},
	{"HAMID", "Hamid Ltd.", 47.48}, {"METROSPIN", "Metrospin Ltd.", 235.16},
	{"QUEENSOUTH", "Queensouth Ltd.", 597.71}, {"SONALIAAAP", "Sonaliaaap Ltd.", 322.18},
	{"GREENDELT", "Greendelt Ltd.", 582.94}, {"PRAGATIINS", "Pragatiins Ltd.", 517.86},
	{"PIONEERINS", "Pioneerins Ltd.", 16.77}, {"RELIANCINS", "Reliancins Ltd.", 435.23},
	{"EASTLAND", "Eastland Ltd.", 412.21}, {"GLOBALINS", "Globalins Ltd.", 326.81},
	{"ISLAMIINS", "Islamiins Ltd.", 167.43}, {"ASIAINS", "Asiains Ltd.", 388.17},
	{"CENTRALINS", "Centralins Ltd.", 75.82}, {"CITYGENINS", "Citygenins Ltd.", 266.51},
	{"PROVATIINS", "Provatiins Ltd.", 277.70}, {"RUPALIINS", "Rupaliins Ltd.", 572.75},
	{"KARNAPHULI", "Karnaphuli Ltd.", 526.75}, {"NITOLINS", "Nitolins Ltd.", 165.40},
	{"PURABIGEN", "Purabigen Ltd.", 305.35}, {"BGIC", "Bgic Ltd.", 115.40},
	{"TAKAFULINS", "Takafulins Ltd.", 548.45}, {"MERCINS", "Mercins Ltd.", 523.61},
	{"PHENIXINS", "Phenixins Ltd.", 186.08}, {"JANATAINS", "Janatains Ltd.", 386.98},
	{"FEDERALINS", "Federalins Ltd.", 369.29}, {"EASTERNINS", "Easternins Ltd.", 100.18},
	{"DHAKAINS", "Dhakains Ltd.", 459.88}, {"DELTALIFE", "Deltalife Ltd.", 328.23},
	{"MEGHNALIFE", "Meghnalife Ltd.", 469.39}, {"NATLIFEINS", "Natlifeins Ltd.", 322.91},
	{"POPULARLIF", "Popularlif Ltd.", 10.34}, {"PRAGATILIF", "Pragatilif Ltd.", 201.25},
	{"PRIMELIFE", "Primelife Ltd.", 21.49}, {"FAREASTLIF", "Fareastlif Ltd.", 558.17},
	{"SUNLIFEINS", "Sunlifeins Ltd.", 528.45}, {"PADMALIFE", "Padmalife Ltd.", 500.68},
	{"PROGRESLIF", "Progreslif Ltd.", 191.43}, {"SANDHANINS", "Sandhanins Ltd.", 44.18},
	{"BNICL", "Bnicl Ltd.", 528.03}, {"AGRANINS", "Agranins Ltd.", 568.70},
	{"CRYSTALINS", "Crystalins Ltd.", 60.54}, {"CONTININS", "Continins Ltd.", 296.73},
	{"LANKABAFIN", "Lankabafin Ltd.", 50.84}, {"IDLC", "Idlc Ltd.", 458.76},
	{"ICB", "Icb Ltd.", 461.84}, {"IPDC", "Ipdc Ltd.", 85.75},
	{"UNITEDFIN", "Unitedfin Ltd.", 290.42}, {"UTTARAFIN", "Uttarafin Ltd.", 334.38},
	{"BAYLEASING", "Bayleasing Ltd.", 166.38}, {"PHOENIXFIN", "Phoenixfin Ltd.", 524.74},
	{"GSPFINANCE", "Gspfinance Ltd.", 259.65}, {"PREMIERLEA", "Premierlea Ltd.", 134.96},
	{"PRIMEFIN", "Primefin Ltd.", 328.18}, {"MIDASFIN", "Midasfin Ltd.", 440.66},
	{"FASFIN", "Fasfin Ltd.", 128.68}, {"BIFC", "Bifc Ltd.", 193.91},
	{"NHFIL", "Nhfil Ltd.", 597.14}, {"ISLAMICFIN", "Islamicfin Ltd.", 393.43},
	{"DELTALIFE2", "Deltalife2 Ltd.", 268.48}, {"UNIONCAP", "Unioncap Ltd.", 315.37},
	{"FLEASEINT", "Fleaseint Ltd.", 81.39}, {"AGNISYSL", "Agnisysl Ltd.", 142.57},
	{"BSRMLTD", "Bsrmltd Ltd.", 209.47}, {"BSRMSTEEL", "Bsrmsteel Ltd.", 357.10},
	{"GPHISPAT", "Gphispat Ltd.", 145.77}, {"RSRMSTEEL", "Rsrmsteel Ltd.", 139.93},
	{"KDSALTD", "Kdsaltd Ltd.", 51.89}, {"WESTERNMAR", "Westernmar Ltd.", 382.35},
	{"RANFOUNDRY", "Ranfoundry Ltd.", 145.08}, {"NPOLYMER", "Npolymer Ltd.", 544.20},
	{"BENGALWTL", "Bengalwtl Ltd.", 517.18}, {"NAVANACNG", "Navanacng Ltd.", 51.81},
	{"IFADAUTOS", "Ifadautos Ltd.", 150.42}, {"ANWARGALV", "Anwargalv Ltd.", 404.70},
	{"MONNOCERA", "Monnocera Ltd.", 136.40}, {"RAKCERAMIC", "Rakceramic Ltd.", 88.06},
	{"FUWANGCER", "Fuwangcer Ltd.", 561.95}, {"SPCERAMICS", "Spceramics Ltd.", 346.92},
	{"STANCERAM", "Stanceram Ltd.", 288.88}, {"BDLAMPS", "Bdlamps Ltd.", 472.93},
	{"ATLASBANG", "Atlasbang Ltd.", 486.42}, {"AZIZPIPES", "Azizpipes Ltd.", 122.34},
	{"EASTERNCAB", "Easterncab Ltd.", 67.19}, {"GOLDENSON", "Goldenson Ltd.", 264.32},
	{"OIMEX", "Oimex Ltd.", 259.91}, {"SINGERBD", "Singerbd Ltd.", 285.54},
	{"NTLTUBES", "Ntltubes Ltd.", 440.15}, {"AFTABAUTO", "Aftabauto Ltd.", 407.29},
	{"QUASEMIND", "Quasemind Ltd.", 590.66}, {"BBS", "Bbs Ltd.", 68.07},
	{"DOMINAGE", "Dominage Ltd.", 247.55}, {"OLYMPIC", "Olympic Ltd.", 210.19},
	{"APEXFOODS", "Apexfoods Ltd.", 518.39}, {"RDFOOD", "Rdfood Ltd.", 156.71},
	{"FINEFOODS", "Finefoods Ltd.", 122.22}, {"BDTHAI", "Bdthai Ltd.", 274.68},
	{"AMANFEED", "Amanfeed Ltd.", 258.91}, {"INDEXAGRO", "Indexagro Ltd.", 174.34},
	{"NATLFEEDML", "Natlfeedml Ltd.", 157.39}, {"FUWANGFOOD", "Fuwangfood Ltd.", 554.73},
	{"EMERALDOIL", "Emeraldoil Ltd.", 271.45}, {"BEACHHATCH", "Beachhatch Ltd.", 518.20},
	{"AGRICMARK", "Agricmark Ltd.", 334.69}, {"GHAIL", "Ghail Ltd.", 39.85},
	{"ZEALBANGLA", "Zealbangla Ltd.", 599.58}, {"SHYAMPSUG", "Shyampsug Ltd.", 503.26},
	{"ZAHEENSPIN", "Zaheenspin Ltd.", 581.71}, {"BANGAS", "Bangas Ltd.", 556.56},
	{"MEGCONMILK", "Megconmilk Ltd.", 510.73}, {"GEMINI2", "Gemini2 Ltd.", 108.12},
	{"LOVELLO", "Lovello Ltd.", 296.53}, {"CONFIDCEM", "Confidcem Ltd.", 136.11},
	{"MEGHNACEM", "Meghnacem Ltd.", 246.61}, {"PREMIERCEM", "Premiercem Ltd.", 44.59},
	{"ARAMITCEM", "Aramitcem Ltd.", 233.59}, {"HEIDELBCEM", "Heidelbcem Ltd.", 591.33},
	{"CROWNCEMNT", "Crowncemnt Ltd.", 166.47}, {"MICEMENT", "Micement Ltd.", 472.60},
	{"BDCOM", "Bdcom Ltd.", 278.45}, {"AAMRANET", "Aamranet Ltd.", 259.57},
	{"AAMRATECH", "Aamratech Ltd.", 574.82}, {"GENEXIL", "Genexil Ltd.", 597.30},
	{"INTECH", "Intech Ltd.", 337.90}, {"AGNI2", "Agni2 Ltd.", 433.86},
	{"ITCL", "Itcl Ltd.", 101.33}, {"DAFODILCOM", "Dafodilcom Ltd.", 185.06},
	{"EGENERATION", "Egeneration Ltd.", 581.54}, {"BEXTECH", "Bextech Ltd.", 351.72},
	{"EASTERNLUB", "Easternlub Ltd.", 329.90}, {"GQBALLPEN", "Gqballpen Ltd.", 451.31},
	{"SAVAREFR", "Savarefr Ltd.", 43.73}, {"USMANIAGL", "Usmaniagl Ltd.", 354.66},
	{"APEXTANRY", "Apextanry Ltd.", 306.68}, {"BATASHOE", "Batashoe Ltd.", 513.10},
	{"LEGACYFOOT", "Legacyfoot Ltd.", 102.89}, {"FORTUNE", "Fortune Ltd.", 576.86},
	{"SAMATALETH", "Samataleth Ltd.", 57.27}, {"NORTHERN", "Northern Ltd.", 119.64},
	{"JUTESPINN", "Jutespinn Ltd.", 361.07}, {"SONALIANSH", "Sonaliansh Ltd.", 408.38},
	{"SONALIPAPR", "Sonalipapr Ltd.", 148.77}, {"HAKKANIPUL", "Hakkanipul Ltd.", 80.73},
	{"PAPERPROC", "Paperproc Ltd.", 535.27}, {"KHULNAPRIN", "Khulnaprin Ltd.", 155.27},
	{"SEAPEARL", "Seapearl Ltd.", 360.77}, {"PENINSULA", "Peninsula Ltd.", 375.44},
	{"UNIQUEHOTL", "Uniquehotl Ltd.", 257.34}, {"BDSERVICE", "Bdservice Ltd.", 354.37},
	{"BERGERPBL", "Bergerpbl Ltd.", 318.44}, {"LINDEBD2", "Lindebd2 Ltd.", 561.48},
	{"GHCL", "Ghcl Ltd.", 130.51}, {"ARAMIT", "Aramit Ltd.", 432.55},
	{"KAY", "Kay Ltd.", 150.82}, {"BEXGSUKUK", "Bexgsukuk Ltd.", 243.51},
	{"STYLECRAFT", "Stylecraft Ltd.", 406.30}, {"BEACON2", "Beacon2 Ltd.", 187.00},
	{"ADNTEL", "Adntel Ltd.", 196.54}, {"AAMRA2", "Aamra2 Ltd.", 453.60},
	{"ACMEPL", "Acmepl Ltd.", 52.80}, {"RANGPURFO", "Rangpurfo Ltd.", 280.39},
	{"MONNOFABR", "Monnofabr Ltd.", 599.09}, {"KHULNAPRN", "Khulnaprn Ltd.", 597.70},
	{"SALAMCRST", "Salamcrst Ltd.", 53.22}, {"MASTERAGRO", "Masteragro Ltd.", 135.76},
	{"KRISHIBID", "Krishibid Ltd.", 166.47}, {"TUNGHAI", "Tunghai Ltd.", 560.62},
	{"MEGHNAMILK", "Meghnamilk Ltd.", 529.71}, {"PHARMA001", "Pharma Industries 1", 792.31},
	{"POWER002", "Power Industries 2", 337.62}, {"TEX003", "Tex Industries 3", 148.71},
	{"INS004", "Ins Industries 4", 751.70}, {"FIN005", "Fin Industries 5", 635.56},
	{"ENG006", "Eng Industries 6", 553.62}, {"FOOD007", "Food Industries 7", 888.61},
	{"CERAMIC008", "Ceramic Industries 8", 591.35}, {"IT009", "It Industries 9", 14.98},
	{"JUTE010", "Jute Industries 10", 736.86}, {"PAPER011", "Paper Industries 11", 275.05},
	{"FUEL012", "Fuel Industries 12", 599.74}, {"SERV013", "Serv Industries 13", 845.53},
	{"BANK014", "Bank Industries 14", 127.79}, {"PHARMA015", "Pharma Industries 15", 110.96},
	{"POWER016", "Power Industries 16", 103.48}, {"TEX017", "Tex Industries 17", 501.48},
	{"INS018", "Ins Industries 18", 250.93}, {"FIN019", "Fin Industries 19", 547.51},
	{"ENG020", "Eng Industries 20", 648.11}, {"FOOD021", "Food Industries 21", 189.61},
	{"CERAMIC022", "Ceramic Industries 22", 573.74}, {"IT023", "It Industries 23", 243.47},
	{"JUTE024", "Jute Industries 24", 443.77}, {"PAPER025", "Paper Industries 25", 815.56},
	{"FUEL026", "Fuel Industries 26", 762.72}, {"SERV027", "Serv Industries 27", 90.33},
	{"BANK028", "Bank Industries 28", 385.83}, {"PHARMA029", "Pharma Industries 29", 254.80},
	{"POWER030", "Power Industries 30", 11.16}, {"TEX031", "Tex Industries 31", 695.84},
	{"INS032", "Ins Industries 32", 576.31}, {"FIN033", "Fin Industries 33", 241.66},
	{"ENG034", "Eng Industries 34", 669.18}, {"FOOD035", "Food Industries 35", 500.10},
	{"CERAMIC036", "Ceramic Industries 36", 389.50}, {"IT037", "It Industries 37", 16.63},
	{"JUTE038", "Jute Industries 38", 75.12}, {"PAPER039", "Paper Industries 39", 795.73},
	{"FUEL040", "Fuel Industries 40", 814.30}, {"SERV041", "Serv Industries 41", 494.67},
	{"BANK042", "Bank Industries 42", 752.46}, {"PHARMA043", "Pharma Industries 43", 527.60},
	{"POWER044", "Power Industries 44", 140.10}, {"TEX045", "Tex Industries 45", 121.68},
	{"INS046", "Ins Industries 46", 282.97}, {"FIN047", "Fin Industries 47", 809.89},
	{"ENG048", "Eng Industries 48", 718.14}, {"FOOD049", "Food Industries 49", 775.75},
	{"CERAMIC050", "Ceramic Industries 50", 809.84}, {"IT051", "It Industries 51", 195.39},
	{"JUTE052", "Jute Industries 52", 230.58}, {"PAPER053", "Paper Industries 53", 99.69},
	{"FUEL054", "Fuel Industries 54", 703.86}, {"SERV055", "Serv Industries 55", 796.65},
	{"BANK056", "Bank Industries 56", 370.49}, {"PHARMA057", "Pharma Industries 57", 561.63},
	{"POWER058", "Power Industries 58", 145.86}, {"TEX059", "Tex Industries 59", 837.45},
	{"INS060", "Ins Industries 60", 779.23}, {"FIN061", "Fin Industries 61", 878.78},
	{"ENG062", "Eng Industries 62", 731.21}, {"FOOD063", "Food Industries 63", 794.22},
	{"CERAMIC064", "Ceramic Industries 64", 30.11}, {"IT065", "It Industries 65", 665.02},
	{"JUTE066", "Jute Industries 66", 304.31}, {"PAPER067", "Paper Industries 67", 838.29},
	{"FUEL068", "Fuel Industries 68", 723.59}, {"SERV069", "Serv Industries 69", 778.75},
	{"BANK070", "Bank Industries 70", 731.19}, {"PHARMA071", "Pharma Industries 71", 245.99},
	{"POWER072", "Power Industries 72", 710.34}, {"TEX073", "Tex Industries 73", 104.42},
	{"INS074", "Ins Industries 74", 785.97}, {"FIN075", "Fin Industries 75", 773.87},
	{"ENG076", "Eng Industries 76", 206.41}, {"FOOD077", "Food Industries 77", 736.40},
	{"CERAMIC078", "Ceramic Industries 78", 418.59}, {"IT079", "It Industries 79", 280.23},
	{"JUTE080", "Jute Industries 80", 717.45}, {"PAPER081", "Paper Industries 81", 211.02},
	{"FUEL082", "Fuel Industries 82", 29.11}, {"SERV083", "Serv Industries 83", 180.27},
	{"BANK084", "Bank Industries 84", 300.81}, {"PHARMA085", "Pharma Industries 85", 779.00},
	{"POWER086", "Power Industries 86", 870.47}, {"TEX087", "Tex Industries 87", 256.98},
	{"INS088", "Ins Industries 88", 580.20}, {"FIN089", "Fin Industries 89", 364.51},
	{"ENG090", "Eng Industries 90", 883.19}, {"FOOD091", "Food Industries 91", 486.30},
	{"CERAMIC092", "Ceramic Industries 92", 845.80}, {"IT093", "It Industries 93", 110.88},
	{"JUTE094", "Jute Industries 94", 873.60}, {"PAPER095", "Paper Industries 95", 167.28},
	{"FUEL096", "Fuel Industries 96", 866.58}, {"SERV097", "Serv Industries 97", 244.80},
	{"BANK098", "Bank Industries 98", 104.70}, {"PHARMA099", "Pharma Industries 99", 395.63},
	{"POWER100", "Power Industries 100", 657.86}, {"TEX101", "Tex Industries 101", 287.80},
	{"INS102", "Ins Industries 102", 548.74}, {"FIN103", "Fin Industries 103", 464.19},
	{"ENG104", "Eng Industries 104", 351.59}, {"FOOD105", "Food Industries 105", 522.32},
	{"CERAMIC106", "Ceramic Industries 106", 235.21}, {"IT107", "It Industries 107", 640.24},
	{"JUTE108", "Jute Industries 108", 9.51}, {"PAPER109", "Paper Industries 109", 833.61},
	{"FUEL110", "Fuel Industries 110", 488.30}, {"SERV111", "Serv Industries 111", 649.73},
	{"BANK112", "Bank Industries 112", 669.82}, {"PHARMA113", "Pharma Industries 113", 606.20},
	{"POWER114", "Power Industries 114", 332.89}, {"TEX115", "Tex Industries 115", 70.42},
	{"INS116", "Ins Industries 116", 600.50}, {"FIN117", "Fin Industries 117", 302.54},
	{"ENG118", "Eng Industries 118", 288.01}, {"FOOD119", "Food Industries 119", 764.43},
	{"CERAMIC120", "Ceramic Industries 120", 650.02}, {"IT121", "It Industries 121", 275.89},
	{"JUTE122", "Jute Industries 122", 283.88}, {"PAPER123", "Paper Industries 123", 372.29},
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8010"
	}
	bus := NewBus()
	eng := NewEngine(loadUniverse(), bus)
	go eng.run()
	eng.startBots(time.Now().UnixNano())

	// FIX acceptor — brokers' fix_initiator_go connect here (real FIX round-trip)
	fixPort := os.Getenv("FIX_PORT")
	if fixPort == "" {
		fixPort = "5001"
	}
	fixSender := os.Getenv("FIX_SENDER")
	if fixSender == "" {
		fixSender = "DSEME"
	}
	if _, err := StartFixAcceptor(eng, fixPort, fixSender); err != nil {
		log.Printf("[FIX] acceptor start failed: %v (HTTP still serving)", err)
	}
	meItchAuth = loadItchAuth()

	mux := http.NewServeMux()
	registerAdmin(mux, eng) // /admin console (login-protected)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		eng.mu.RLock()
		phase, running := eng.phase, eng.running
		eng.mu.RUnlock()
		// Public gateway health for the exchange /marketfeed panel. No secrets:
		// if this responds, the gateway is up and serving all feeds. `phase` and
		// `running` reflect the live matching-engine trading state.
		writeJSON(w, map[string]any{
			"ok": true, "phase": phase, "running": running,
			"feeds": map[string]bool{"market": true, "index": true, "news": true, "depth": true},
		})
	})
	itch := NewItchGateway(eng)
	go itch.run()
	go newItchFeed(eng).run()  // native DSE-ITCH SoupBinTCP feed for the C++ itch service
	mux.HandleFunc("/market", itch.serve(itch.market, selMarket))
	mux.HandleFunc("/marketdepth", itch.serve(itch.depth, selDepth))
	mux.HandleFunc("/index", itch.serve(itch.index, selIndex))
	pool, poolErr := mustPool()
	if poolErr != nil {
		log.Printf("[bench] pool init: %v", poolErr)
	}
	bench := NewBench(pool, eng)
	mux.HandleFunc("/bench", serveBenchUI)
	mux.HandleFunc("/api/bench/start", func(w http.ResponseWriter, r *http.Request) {
		cfg := BenchConfig{RMS: true, Persist: true}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&cfg)
		}
		if err := bench.Start(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "cfg": cfg})
	})
	mux.HandleFunc("/api/bench/stop", func(w http.ResponseWriter, r *http.Request) {
		bench.Stop()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/bench/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, bench.Stats())
	})
	live := NewLiveBench(eng)
	mux.HandleFunc("/api/bots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"paused": atomic.LoadInt32(&botsPaused) == 1})
	})
	mux.HandleFunc("/api/bots/pause", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&botsPaused, 1)
		writeJSON(w, map[string]any{"paused": true})
	})
	mux.HandleFunc("/api/bots/resume", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&botsPaused, 0)
		writeJSON(w, map[string]any{"paused": false})
	})
	// Session control: pre-open call auction → continuous trading.
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.phase})
	})
	mux.HandleFunc("/api/session/preopen", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.SetPhase("Pre-Open")})
	})
	mux.HandleFunc("/api/session/open", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"phase": eng.SetPhase("Continuous Trading")})
	})
	mux.HandleFunc("/api/live/start", func(w http.ResponseWriter, r *http.Request) {
		var cfg LiveConfig
		if r.Body != nil { json.NewDecoder(r.Body).Decode(&cfg) }
		if err := live.Start(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()}); return
		}
		writeJSON(w, map[string]any{"ok": true, "cfg": cfg})
	})
	mux.HandleFunc("/api/live/stop", func(w http.ResponseWriter, r *http.Request) {
		live.Stop(); writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/live/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, live.Stats())
	})
	mux.HandleFunc("/api/live/latency", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, pipelineLatency())
	})
	mux.HandleFunc("/api/live/cleanup", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, live.Cleanup())
	})
	mux.HandleFunc("/api/bench/summary.csv", func(w http.ResponseWriter, r *http.Request) {
		bench.writeSummaryCSV(w)
	})
	mux.HandleFunc("/api/bench/orders.csv", ordersCSVHandler(pool))
	mux.HandleFunc("/api/loadtest", func(w http.ResponseWriter, r *http.Request) {
		n := atoiDefault(r.URL.Query().Get("n"), 100000)
		if n > 2000000 {
			n = 2000000
		}
		writeJSON(w, eng.LoadTest(n, r.URL.Query().Get("symbol")))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })

	mux.HandleFunc("/api/market", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"index": eng.index, "phase": eng.phase, "rows": eng.marketRows()})
	})
	mux.HandleFunc("/api/book", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.snapshotBook(r.URL.Query().Get("symbol")))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		eng.mu.RLock()
		st := map[string]any{"phase": eng.phase, "index": eng.index, "instruments": len(eng.instr), "orders": eng.seq, "trades": eng.tradeSeq, "running": eng.running}
		eng.mu.RUnlock()
		writeJSON(w, st)
	})
	// external order entry (this is where the OMS / fix acceptor will feed orders)
	mux.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		var req struct {
			Symbol string  `json:"symbol"`
			Side   string  `json:"side"`
			Type   string  `json:"type"`
			Price  float64 `json:"price"`
			Qty    int64   `json:"qty"`
			Actor  string  `json:"actor"`
			ClOrdID string `json:"cl_ord_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Qty <= 0 || eng.md[req.Symbol] == nil {
			http.Error(w, "invalid order", 400)
			return
		}
		side := Buy
		if req.Side == "SELL" || req.Side == "sell" {
			side = Sell
		}
		typ := Limit
		if req.Type == "MARKET" || req.Type == "market" || req.Price <= 0 {
			typ = Market
		}
		actor := req.Actor
		if actor == "" {
			actor = "API"
		}
		o := eng.newOrder(req.Symbol, side, typ, req.Price, req.Qty, actor)
		o.ClOrdID = req.ClOrdID
		rep := eng.SubmitAndWait(o)
		writeJSON(w, rep)
	})

	// /events is dual-mode: a WebSocket upgrade (the trader SPA news socket)
	// routes to the itch news/time-&-sales feed; a plain GET is the sim UI's
	// SSE market stream.
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			itch.serveEvents(w, r)
			return
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		ch := bus.subscribe()
		defer bus.unsubscribe(ch)
		fmt.Fprintf(w, "retry: 3000\n\n")
		fl.Flush()
		ka := time.NewTicker(15 * time.Second)
		defer ka.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				fl.Flush()
			case <-ka.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				fl.Flush()
			}
		}
	})

	log.Printf("Velexa Simulator Matching Engine on :%s  (%d instruments, phase=%s)", port, len(universe), eng.phase)
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}
