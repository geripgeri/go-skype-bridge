package main

import (
	"fmt"
	skypeExt "github.com/kelaresg/matrix-skype/skype-ext"
	"html"
	"regexp"
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"github.com/kelaresg/matrix-skype/types"
)

var italicRegex = regexp.MustCompile("([\\s>~*]|^)_(.+?)_([^a-zA-Z\\d]|$)")
var boldRegex = regexp.MustCompile("([\\s>_~]|^)\\*(.+?)\\*([^a-zA-Z\\d]|$)")
var strikethroughRegex = regexp.MustCompile("([\\s>_*]|^)~(.+?)~([^a-zA-Z\\d]|$)")
var codeBlockRegex = regexp.MustCompile("```(?:.|\n)+?```")
//var mentionRegex = regexp.MustCompile("@[0-9]+")
//var mentionRegex = regexp.MustCompile("@(.*)")
var mentionRegex = regexp.MustCompile("<at[^>]+\\bid=\"([^\"]+)\"(.*?)</at>*")

type Formatter struct {
	bridge *Bridge

	matrixHTMLParser *format.HTMLParser

	waReplString   map[*regexp.Regexp]string
	waReplFunc     map[*regexp.Regexp]func(string) string
	waReplFuncText map[*regexp.Regexp]func(string) string
}

func NewFormatter(bridge *Bridge) *Formatter {
	formatter := &Formatter{
		bridge: bridge,
		matrixHTMLParser: &format.HTMLParser{
			TabsToSpaces: 4,
			Newline:      "\n",

			PillConverter: func(mxid, eventID string) string {
				if mxid[0] == '@' {
					puppet := bridge.GetPuppetByMXID(id.UserID(mxid))
					if puppet != nil {
						return "@" + puppet.PhoneNumber()
					}
				}
				return mxid
			},
			BoldConverter: func(text string) string {
				return fmt.Sprintf("*%s*", text)
			},
			ItalicConverter: func(text string) string {
				return fmt.Sprintf("_%s_", text)
			},
			StrikethroughConverter: func(text string) string {
				return fmt.Sprintf("~%s~", text)
			},
			MonospaceConverter: func(text string) string {
				return fmt.Sprintf("```%s```", text)
			},
			MonospaceBlockConverter: func(text, language string) string {
				return fmt.Sprintf("```%s```", text)
			},
		},
		waReplString: map[*regexp.Regexp]string{
			italicRegex:        "$1<em>$2</em>$3",
			boldRegex:          "$1<strong>$2</strong>$3",
			strikethroughRegex: "$1<del>$2</del>$3",
		},
	}
	formatter.waReplFunc = map[*regexp.Regexp]func(string) string{
		codeBlockRegex: func(str string) string {
			str = str[3 : len(str)-3]
			if strings.ContainsRune(str, '\n') {
				return fmt.Sprintf("<pre><code>%s</code></pre>", str)
			}
			return fmt.Sprintf("<code>%s</code>", str)
		},
		mentionRegex: func(str string) string {
			mxid, displayname := formatter.getMatrixInfoByJID(str[1:] + skypeExt.NewUserSuffix)
			return fmt.Sprintf(`<a href="https://matrix.to/#/%s">%s</a>`, mxid, displayname)
		},
	}
	formatter.waReplFuncText = map[*regexp.Regexp]func(string) string{
		mentionRegex: func(str string) string {
			r := regexp.MustCompile(`<at[^>]+\bid="([^"]+)"(.*?)</at>*`)
			matches := r.FindAllStringSubmatch(str, -1)
			displayname := ""
			var mxid id.UserID
			if len(matches) > 0 {
				for _, match := range matches {
					mxid, displayname = formatter.getMatrixInfoByJID(match[1] + skypeExt.NewUserSuffix)
				}
			}
			//mxid, displayname := formatter.getMatrixInfoByJID(str[1:] + whatsappExt.NewUserSuffix)
			return fmt.Sprintf(`<a href="https://matrix.to/#/%s">%s</a>`, mxid, displayname)
			// _, displayname = formatter.getMatrixInfoByJID(str[1:] + whatsappExt.NewUserSuffix)
			//fmt.Println("ParseWhatsAp4", displayname)
			//return displayname
		},
	}
	return formatter
}

func (formatter *Formatter) getMatrixInfoByJID(jid types.SkypeID) (mxid id.UserID, displayname string) {
	if user := formatter.bridge.GetUserByJID(jid); user != nil {
		mxid = user.MXID
		displayname = string(user.MXID)
	} else if puppet := formatter.bridge.GetPuppetByJID(jid); puppet != nil {
		mxid = puppet.MXID
		displayname = puppet.Displayname
	}
	return
}

func (formatter *Formatter) ParseSkype(content *event.MessageEventContent) {
	output := html.EscapeString(content.Body)
	for regex, replacement := range formatter.waReplString {
		output = regex.ReplaceAllString(output, replacement)
	}
	for regex, replacer := range formatter.waReplFunc {
		output = regex.ReplaceAllStringFunc(output, replacer)
	}
	if output != content.Body {
		output = strings.Replace(output, "\n", "<br/>", -1)

		// parse @user message
		r := regexp.MustCompile(`<at[^>]+\bid="([^"]+)"(.*?)</at>*`)
		matches := r.FindAllStringSubmatch(content.Body, -1)
		displayname := ""
		var mxid id.UserID
		if len(matches) > 0 {
			for _, match := range matches {
				mxid, displayname = formatter.getMatrixInfoByJID(match[1] + skypeExt.NewUserSuffix)
				content.FormattedBody = strings.ReplaceAll(content.Body, match[0], fmt.Sprintf(`<a href="https://matrix.to/#/%s">%s</a>`, mxid, displayname))
				content.Body = content.FormattedBody
			}
		}

		// parse quote message
		content.Body = strings.ReplaceAll(content.Body, "\n", "")
		quoteReg := regexp.MustCompile(`<quote[^>]+\bauthor="([^"]+)" authorname="([^"]+)" timestamp="([^"]+)".*>.*?</legacyquote>(.*?)<legacyquote>.*?</legacyquote></quote>(.*)`)
		quoteMatches := quoteReg.FindAllStringSubmatch(content.Body, -1)
		if len(quoteMatches) > 0 {
			for _, match := range quoteMatches {
				mxid, displayname = formatter.getMatrixInfoByJID("8:" + match[1] + skypeExt.NewUserSuffix)
				//href1 := fmt.Sprintf(`https://matrix.to/#/!kpouCkfhzvXgbIJmkP:oliver.matrix.host/$fHQNRydqqqAVS8usHRmXn0nIBM_FC-lo2wI2Uol7wu8?via=oliver.matrix.host`)
				href1 := ""
				//mxid `@skype&8-live-xxxxxx:name.matrix.server`
				href2 := fmt.Sprintf(`https://matrix.to/#/%s`, mxid)
				newContent := fmt.Sprintf(`<mx-reply><blockquote><a href="%s"></a> <a href="%s">%s</a><br>%s</blockquote></mx-reply>%s`,
					href1,
					href2,
					mxid,
					match[4],
					match[5])
				content.FormattedBody = newContent
				content.Body = match[4] + "\n" + match[5]
			}
		}

		content.Format = event.FormatHTML
	}
}

func (formatter *Formatter) ParseMatrix(html string) string {
	return formatter.matrixHTMLParser.Parse(html)
}
