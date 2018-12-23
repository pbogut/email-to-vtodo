package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/veqryn/go-email/email"
)

var opts struct {
	Verbose   bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
	CalPath   string `short:"p" long:"path" description:"Path to calendar folder" required:"true"`
	EmailFile string `short:"f" long:"email-file" description:"Path to email file" required:"true"`
	Type      string `short:"t" long:"type" description:"Message mime type" default:"text/plain"`
	HtmlCmd   string `short:"h" long:"html-cmd" description:"Html parse command" default:"w3m -T text/html %s"`
}

func debug(message string, params ...interface{}) {
	if opts.Verbose {
		fmt.Printf(message+"\n", params...)
	}
}

func html_to_text(html string) string {
	if opts.HtmlCmd == "" {
		return html
	}

	tmpFile, err := ioutil.TempFile(os.TempDir(), "email-to-vtodo-")
	if err != nil {
		log.Fatal("Cannot create temporary file.", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(html)

	htmlCmd := fmt.Sprintf(opts.HtmlCmd, tmpFile.Name())
	cmd := exec.Command("sh", "-c", htmlCmd)
	out, _ := cmd.Output()

	return string(out)
}

func email_file_to_msg(file_path string) *email.Message {
	file, _ := ioutil.ReadFile(file_path)
	reader := strings.NewReader(string(file))
	msg, _ := email.ParseMessage(reader)

	return msg
}

// this is lazy way to decode subject,
// it will break at some emails, I'm sure
func subject_decode(name string) string {
	re := regexp.MustCompile("^=\\?[a-zA-Z0-9_\\-]*\\?.\\?(.*)\\?=")
	newName := re.ReplaceAllString(name, "$1")
	if newName != name {
		re = regexp.MustCompile("=([A-F0-9][A-F0-9])")
		newName = re.ReplaceAllString(newName, "%$1")
		newName, _ = url.PathUnescape(newName)
	}
	return newName
}

func get_uuid() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return uuid
}

func get_description(msg *email.Message) string {
	var body string
	var plain string
	var html string

	for _, part := range msg.MessagesAll() {
		mediaType, _, _ := part.Header.ContentType()
		switch mediaType {
		case "text/html":
			html = html_to_text(string(part.Body))
			if body == "" {
				body = html
			}
		case "text/plain":
			plain = string(part.Body)
			if body == "" {
				body = plain
			}
		}
	}

	if html != "" && opts.Type == "text/html" {
		body = html
	}
	if plain != "" && opts.Type == "text/plain" {
		body = plain
	}

	return strings.Replace(body, "\n", "\\n", -1)
}

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		return
	}

	uuid := get_uuid()
	msg := email_file_to_msg(opts.EmailFile)

	icsVtodo := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Nextcloud Tasks vundefined
BEGIN:VTODO
CREATED;VALUE=DATE-TIME:{{.Created}}
DESCRIPTION:{{.Description}}
DTSTAMP;VALUE=DATE-TIME:{{.Dtstamp}}
LAST-MODIFIED;VALUE=DATE-TIME:{{.Created}}
PERCENT-COMPLETE:0
PRIORITY:0
SEQUENCE:6
STATUS:NEEDS-ACTION
SUMMARY:{{.Summary}}
UID:{{.Uuid}}
X-OC-HIDESUBTASKS:0
END:VTODO
END:VCALENDAR`

	description := get_description(msg)

	date, _ := msg.Header.Date()
	created := time.Now()
	m := map[string]interface{}{
		"Summary":     html.EscapeString(subject_decode(msg.Header.Subject())),
		"Dtstamp":     date.Format("20060102T150405Z"),
		"Created":     created.Format("20060102T150405Z"),
		"Description": description,
		"Uuid":        uuid,
	}

	content := new(bytes.Buffer)
	templ, _ := template.New("preview").Parse(icsVtodo)
	templ.Execute(content, m)

	dir := opts.CalPath
	fileName := dir + "/" + uuid + ".ics"

	err = ioutil.WriteFile(fileName, content.Bytes(), 0777)
	if err != nil {
		log.Fatal("Cannot create todo file.\n", err)
	}
}
