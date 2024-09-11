package main

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "embed"

	"golang.org/x/net/html"

	"cloud.google.com/go/storage"
)

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		// https://cloud.google.com/logging/docs/structured-logging
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			if a.Key == slog.LevelKey {
				a.Key = "severity"
				level := a.Value.Any().(slog.Level)
				if level == slog.LevelWarn {
					a.Value = slog.StringValue("WARNING")
				}
			}
			return a
		},
	})))
}

const (
	icalDateFormat     = "20060102"
	icalDateTimeFormat = "20060102T150405"
)

type icalEvent struct {
	Uid       string
	Summary   string
	IsAllDay  bool
	StartDate string
	StartTime string
	EndTime   string
}

//go:embed schedule.ics.go.tpl
var icalTpl string

func main() {
	url := os.Getenv("SCHEDULE_URL")
	if url == "" {
		url = "https://aikatsu-academy.com/schedule/"
	}

	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		slog.Error("failed to get schedule page")
		panic(err)
	}

	doc, _ := html.Parse(resp.Body)

	// find every day schedule
	sb := findScheduleElement(doc, "p-schedule-body")
	if sb == nil {
		slog.Error("not found schedule body")
		panic("panic")
	}
	slog.Info("dump", "node", sb)

	var cal [][][]string
	mc := -1

	for _, item := range findScheduleItem(sb) {
		day, err := strconv.Atoi(findScheduleElement(item, "num").FirstChild.Data)
		if err != nil {
			slog.Error("failed to parse day of month")
			panic(err)
		}
		if day == 1 {
			mc++
			cal = append(cal, [][]string{})
		}
		cal[mc] = append(cal[mc], findEvents(item))
	}

	// find months in schedule
	sh := findScheduleElement(doc, "p-schedule-header")
	if sh == nil {
		slog.Error("not found schedule header")
		panic("panic")
	}
	slog.Info("dump", "node", sh)

	// regexp for streaming event
	// delimiter may be half space " " or full space "　" (in Japanese)
	reForSe := regexp.MustCompile(`(\d+):(\d+)〜[ \x{3000}]?(.*)`)

	mos := findMonth(sh)
	slog.Info("dump", "months", mos)

	var icalEvents []icalEvent

	for i, mo := range mos {
		t, err := time.Parse("2006.1", mo)
		if err != nil {
			panic(err)
		}
		slog.Info("month", "index", i, "time", t)

		// check events for each day
		for day, events := range cal[i] {
			for _, e := range events {
				h := 0 * time.Hour
				m := 0 * time.Minute
				isAllDay := true
				title := e

				r := reForSe.FindStringSubmatch(e)
				if r != nil {
					title = r[3]

					h, err = time.ParseDuration(r[1] + "h")
					if err != nil {
						panic(err)
					}
					m, err = time.ParseDuration(r[2] + "m")
					if err != nil {
						panic(err)
					}
					isAllDay = false
				} else {
					if strings.Contains(e, "「アイカツアカデミー！配信部」デミカツ通信") {
						h = 20 * time.Hour
						isAllDay = false
					}
				}

				st := t.AddDate(0, 0, day).Add(h + m).Format(icalDateTimeFormat)

				hash := sha256.Sum256([]byte("dmkt-schedule" + title + st))

				icalEvents = append(icalEvents, icalEvent{
					Uid:       strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:])),
					Summary:   title,
					IsAllDay:  isAllDay,
					StartDate: t.AddDate(0, 0, day).Format(icalDateFormat),
					StartTime: st,
					EndTime:   t.AddDate(0, 0, day).Add(h + m + 1*time.Hour).Format(icalDateTimeFormat),
				})
				slog.Info("dump", "event", icalEvents[len(icalEvents)-1])
			}
		}
	}

	// write ical into GCS
	gcsBkt, ok := os.LookupEnv("GCS_BUCKET")
	if !ok {
		slog.Error("GCS_BUCKET must be set")
		panic("panic")
	}
	gcsPath, ok := os.LookupEnv("GCS_PATH")
	if !ok {
		slog.Error("GCS_PATH must be set")
		panic("panic")
	}

	ctx := context.Background()
	gcs, err := storage.NewClient(ctx)
	if err != nil {
		slog.Error("failed to init")
		panic(err)
	}
	w := gcs.Bucket(gcsBkt).Object(gcsPath).NewWriter(ctx)
	w.ObjectAttrs.ContentType = "text/calendar"
	defer w.Close()

	// render template
	tpl, err := template.New("ical").Parse(icalTpl)
	if err != nil {
		slog.Error("failed to parse ical template")
		panic(err)
	}
	if err := tpl.Execute(w, map[string]interface{}{"events": icalEvents}); err != nil {
		slog.Error("failed to execute ical template")
		panic(err)
	}

	slog.Info("succeeded!")
}

func findScheduleElement(n *html.Node, class string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Key == "class" && a.Val == class {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r := findScheduleElement(c, class)
		if r != nil {
			return r
		}
	}
	return nil
}

func findMonth(n *html.Node) []string {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Key == "class" && a.Val == "swiper-slide" {
				return []string{n.FirstChild.Data}
			}
		}
	}

	var mos []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r := findMonth(c)
		if r != nil {
			mos = append(mos, r...)
		}
	}

	if len(mos) > 0 {
		return mos
	} else {
		return nil
	}
}

func findScheduleItem(n *html.Node) []*html.Node {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Key == "class" && a.Val == "p-schedule-body__item" {
				return []*html.Node{n}
			}
		}
	}

	var items []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := findScheduleItem(c); r != nil {
			items = append(items, r...)
		}
	}

	if len(items) > 0 {
		return items
	} else {
		return nil
	}
}

func findEvents(n *html.Node) []string {
	if n.Type == html.ElementNode && n.Data == "p" {
		return []string{n.FirstChild.Data}
	}

	var ss []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := findEvents(c); r != nil {
			ss = append(ss, r...)
		}
	}

	if len(ss) > 0 {
		return ss
	} else {
		return nil
	}
}
