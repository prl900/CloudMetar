package hello

import (
	"bufio"
        "fmt"
        "net/http"
        "regexp"
        "time"
        "strconv"

        "google.golang.org/appengine"
        "google.golang.org/appengine/urlfetch"
)

const dateFormat = "2006/01/02 15:04"

var parserStrings map[string]string = map[string]string{"type": `^(?P<type>METAR|SPECI)?\s+`,
	"station":    `^(?P<station>[A-Z][A-Z0-9]{3})\s+`,
	"time":       `^(?P<day>\d\d)(?P<hour>\d\d)(?P<min>\d\d)Z?\s+`,
	"modifier":   `^(?P<mod>AUTO|FINO|NIL|TEST|CORR?|RTD|CC[A-G])\s+`,
	"wind":       `^(?P<dir>\d{3}|0|///|MMM|VRB)(?P<speed>P?[\dO]{2,3}|[/M]{2,3})(?P<gust>G(\d{1,3}|[/M]{1,3}))?(?P<units>KT|KMH|MPS)?(\s+(?P<varfrom>\d\d\d)V(?P<varto>\d\d\d))?\s+`,
	"visibility": `(?P<vis>(?P<dist>(M|P)?\d\d\d\d)(?P<dir>[NSEW][EW]?|NDV)?|(?P<distu>(M|P)?(\d+))(?P<units>SM|KM|M|U)?|CAVOK)\s+`,
	"runway":     `^(RVRNO | R(?P<name>\d\d(RR?|LL?|C)?)/(?P<low>(M|P)?\d\d\d\d)(V(?P<high>(M|P)?\d\d\d\d))?(?P<unit>FT)?[/NDU]*)\s+`,
	"weather":    `(?P<int>(-|\+|VC)+)?(?P<desc>(MI|PR|BC|DR|BL|SH|TS|FZ)+)?(:?(?P<prec>(DZ|RA|SN|SG|IC|PL|GR|GS|UP)+)(?P<obsc>BR|FG|FU|VA|DU|SA|HZ|PY)?(?P<other>PO|SQ|FC|SS|DS)?|(?P<obsc>BR|FG|FU|VA|DU|SA|HZ|PY)(?P<other>PO|SQ|FC|SS|DS)?|(?P<other>PO|SQ|FC|SS|DS))+\s+`,
	"sky":        `(?P<cover>VV|CLR|SKC|SCK|NSC|NCD|BKN|SCT|FEW|OVC)(?P<height>\d{2,4})?(?P<cloud>([A-Z][A-Z]+))?\s+`,
	"temp":       `^(?P<temp>(M|-)?\d+|//|XX|MM)/(?P<dewpt>(M|-)?\d+|//|XX|MM)?\s+`,
	"press":      `^(?P<unit>A|Q|QNH|SLP)?(?P<press>\d{3,4}|////)(?P<unit2>INS)?\s*`}

type Wind struct {
	Vrb     bool
	Dir     int
	Spd     int
	Gust    int
	VarFrom int
	VarTo   int
}

type Weather struct {
	Intens string
	Descr  string
	Precip string
	Other  string
}

type Sky struct {
	Cover  string
	Height int
	Cloud  string
}

type Metar struct {
	Station    string
	Time       time.Time
	Mod        string
	Wind       Wind
	Visibility int
	Weather    []Weather
	Sky        []Sky
	Temp       int
	DewPt      int
	Pressure   int
}

func (m *Metar) Parse(rawMetar, rawDate string) error {
	parsers := map[string]*regexp.Regexp{}
	for key, value := range parserStrings {
		parsers[key] = regexp.MustCompile(value)
	}

	idx := parsers["type"].FindStringSubmatchIndex(rawMetar)
	if idx != nil {
		rawMetar = rawMetar[idx[1]:]
	}

	idx = parsers["station"].FindStringSubmatchIndex(rawMetar)
	if idx == nil {
		return fmt.Errorf("Error parsing station identifier")
	}

	m.Station = rawMetar[idx[2]:idx[3]]
	rawMetar = rawMetar[idx[1]:]

	idx = parsers["time"].FindStringSubmatchIndex(rawMetar)
	if idx == nil {
		return fmt.Errorf("Error parsing metar time")
	}

	t, err := time.Parse(dateFormat, rawDate)
	if err != nil {
		return fmt.Errorf("Error parsing message time")
	}

	var day, hour, min int
	if day, err = strconv.Atoi(rawMetar[idx[2]:idx[3]]); err != nil {
		return fmt.Errorf("Error converting day in metar")
	}
	if hour, err = strconv.Atoi(rawMetar[idx[4]:idx[5]]); err != nil {
		return fmt.Errorf("Error converting hour in metar")
	}
	if min, err = strconv.Atoi(rawMetar[idx[6]:idx[7]]); err != nil {
		return fmt.Errorf("Error converting minute in metar")
	}
	m.Time = time.Date(t.Year(), t.Month(), day, hour, min, 0, 0, time.UTC)
	rawMetar = rawMetar[idx[1]:]

	idx = parsers["modifier"].FindStringSubmatchIndex(rawMetar)
	if idx != nil {
		m.Mod = rawMetar[idx[2]:idx[3]]
		rawMetar = rawMetar[idx[1]:]
	}

	idx = parsers["wind"].FindStringSubmatchIndex(rawMetar)
	if idx == nil {
		return fmt.Errorf("Error parsing metar wind")
	}

	wind := Wind{}
	if idx[2] != -1 && idx[3] != -1 {
		if rawMetar[idx[2]:idx[3]] == "VRB" {
			wind.Vrb = true
		} else {
			if wdir, err := strconv.Atoi(rawMetar[idx[2]:idx[3]]); err == nil {
				wind.Dir = wdir
			} else {
				return fmt.Errorf("Error converting wind direction in metar")
			}
		}
	}
	if idx[4] != -1 && idx[5] != -1 {
		if wspd, err := strconv.Atoi(rawMetar[idx[4]:idx[5]]); err == nil {
			wind.Spd = wspd
		} else {
			return fmt.Errorf("Error converting wind speed in metar")
		}
	}

	if idx[8] != -1 && idx[9] != -1 {
		if gust, err := strconv.Atoi(rawMetar[idx[8]:idx[9]]); err == nil {
			wind.Gust = gust
		} else {
			return fmt.Errorf("Error converting gust speed in metar")
		}
	}

	if idx[14] != -1 && idx[15] != -1 {
		if vfrom, err := strconv.Atoi(rawMetar[idx[14]:idx[15]]); err == nil {
			wind.VarFrom = vfrom
		} else {
			return fmt.Errorf("Error converting variable from wind direction in metar")
		}
	}

	if idx[16] != -1 && idx[17] != -1 {
		if vto, err := strconv.Atoi(rawMetar[idx[16]:idx[17]]); err == nil {
			wind.VarTo = vto
		} else {
			return fmt.Errorf("Error converting variable to wind direction in metar")
		}
	}
	m.Wind = wind
	rawMetar = rawMetar[idx[1]:]

	idx = parsers["visibility"].FindStringSubmatchIndex(rawMetar)
	if idx == nil || idx[4] == -1 || idx[5] == -1 {
		return fmt.Errorf("Error parsing metar visibility")
	}

	if vis, err := strconv.Atoi(rawMetar[idx[4]:idx[5]]); err == nil {
		m.Visibility = vis
	} else {
		return fmt.Errorf("Error converting visibility value in metar")
	}
	rawMetar = rawMetar[idx[1]:]

	idxs := parsers["weather"].FindAllStringSubmatchIndex(rawMetar, -1)
	if idxs != nil {
		for _, idx = range idxs {
			w := Weather{}
			if idx[2] != -1 && idx[3] != -1 {
				w.Intens = rawMetar[idx[2]:idx[3]]
			}
			if idx[6] != -1 && idx[7] != -1 {
				w.Descr = rawMetar[idx[6]:idx[7]]
			}
			if idx[10] != -1 && idx[11] != -1 {
				w.Precip = rawMetar[idx[10]:idx[11]]
			}
			if idx[16] != -1 && idx[17] != -1 {
				w.Other = rawMetar[idx[16]:idx[17]]
			}
			m.Weather = append(m.Weather, w)
		}
		rawMetar = rawMetar[idx[1]:]
	}

	idxs = parsers["sky"].FindAllStringSubmatchIndex(rawMetar, -1)
	if idxs == nil {
		return fmt.Errorf("Error parsing metar sky")
	}
	for _, idx = range idxs {
		sky := Sky{}
		if idx[2] != -1 && idx[3] != -1 {
			sky.Cover = rawMetar[idx[2]:idx[3]]
		}
		if idx[4] != -1 && idx[5] != -1 {
			if height, err := strconv.Atoi(rawMetar[idx[4]:idx[5]]); err == nil {
				sky.Height = height
			} else {
				return fmt.Errorf("Error converting cloud height value in metar")
			}

		}
		if idx[6] != -1 && idx[7] != -1 {
			sky.Cloud = rawMetar[idx[6]:idx[7]]
		}
		m.Sky = append(m.Sky, sky)
	}
	rawMetar = rawMetar[idx[1]:]

	idx = parsers["temp"].FindStringSubmatchIndex(rawMetar)
	if idx == nil {
		return fmt.Errorf("Error parsing metar temperature")
	}

	tempStr := rawMetar[idx[2]:idx[3]]
	if idx[4] != -1 && idx[5] != -1 && rawMetar[idx[4]:idx[5]] == "M" {
		tempStr = "-" + tempStr[1:]
	}
	if temp, err := strconv.Atoi(tempStr); err == nil {
		m.Temp = temp
	} else {
		return fmt.Errorf("Error converting temperature value in metar")
	}

	dewPtStr := rawMetar[idx[6]:idx[7]]
	if idx[8] != -1 && idx[9] != -1 && rawMetar[idx[8]:idx[9]] == "M" {
		dewPtStr = "-" + dewPtStr[1:]
	}
	if dewPt, err := strconv.Atoi(dewPtStr); err == nil {
		m.DewPt = dewPt
	} else {
		return fmt.Errorf("Error converting dew point value in metar")
	}
	rawMetar = rawMetar[idx[1]:]

	idx = parsers["press"].FindStringSubmatchIndex(rawMetar)
	if idx == nil {
		return fmt.Errorf("Error parsing metar pressure")
	}
	if idx[2] != -1 && idx[3] != -1 && rawMetar[idx[2]:idx[3]] == "Q" {
		if value, err := strconv.Atoi(rawMetar[idx[4]:idx[5]]); err == nil {
			m.Pressure = value
		} else {
			return fmt.Errorf("Error converting pressure value in metar")
		}
	} else {
		return fmt.Errorf("Error interpreting metar pressure value")
	}

	return nil
}
func init() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {
	url := "http://tgftp.nws.noaa.gov/data/observations/metar/stations/YSSY.TXT"
        ctx := appengine.NewContext(r)
        client := urlfetch.Client(ctx)
        resp, err := client.Get(url)
        if err != nil {
                http.Error(w, fmt.Sprintf("Err 0: %v", err), 400)
                return
        }
        defer resp.Body.Close()
        br := bufio.NewReader(resp.Body)
        line, _, err := br.ReadLine()
        if err != nil {
                http.Error(w, "Err 1", 400)
                return
        }
        rawDate := string(line)
        line, _, err = br.ReadLine()
        if err != nil {
                http.Error(w, "Err 2", 400)
                return
        }
        rawMetar := string(line)

        m := &Metar{}
	err = m.Parse(rawMetar, rawDate)

	fmt.Fprint(w, fmt.Sprintf("Date: %s Metar: %v Error: %v", rawDate, m, err))
        //fmt.Fprint(w, fmt.Sprintf("Date: %s Metar: %s", rawDate, rawMetar))
}
