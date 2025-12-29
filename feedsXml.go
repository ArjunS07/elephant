package main

import "encoding/xml"
type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Description string `xml:"description"`
	Items []Item `xml:"item"`
}

type Item struct {
	Title string    `xml:"title"`
	Description string    `xml:"description"`
	Enclosure Enclosure `xml:"enclosure"`
	GUID string    `xml:"guid"`
}

type Enclosure struct {
	URL string `xml:"url,attr"`
	Length string `xml:"length,attr"`
	Type string `xml:"type,attr"`
}
