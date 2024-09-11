BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Setuu//dmkt-schedule v0.1//JA
BEGIN:VTIMEZONE
TZID:Asia/Tokyo
BEGIN:STANDARD
DTSTART:19390101T000000
TZOFFSETFROM:+0900
TZOFFSETTO:+0900
END:STANDARD
END:VTIMEZONE
{{- range .events }}
BEGIN:VEVENT
UID:{{ .Uid }}@neigepluie.net
SUMMARY:{{ .Summary }}
TRANSP:OPAQUE
{{- if .IsAllDay }}
DTSTART;TZID=Asia/Tokyo;VALUE=DATE:{{ .StartDate }}
{{- else }}
DTSTART;TZID=Asia/Tokyo:{{ .StartTime }}
DTEND;TZID=Asia/Tokyo:{{ .EndTime }}
{{- end }}
END:VEVENT
{{- end }}
END:VCALENDAR
