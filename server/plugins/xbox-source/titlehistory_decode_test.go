package main

import (
	"encoding/json"
	"testing"
)

func TestTitleHubResponse_decodeSample(t *testing.T) {
	const sample = `{
	  "xuid": "2533274807551358",
	  "titles": [
	    {
	      "titleId": 255867542,
	      "name": "Enter the Gungeon™",
	      "productId": "9P20JCF7BV93",
	      "isStreamable": true,
	      "gamePass": {"isGamePass": false},
	      "devices": ["Win32"]
	    },
	    {
	      "titleId": "12345",
	      "name": "String ID Game",
	      "isStreamable": false
	    }
	  ]
	}`
	var got titleHubResponse
	if err := json.Unmarshal([]byte(sample), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Titles) != 2 {
		t.Fatalf("titles: got %d", len(got.Titles))
	}
	if string(got.Titles[0].TitleID) != "255867542" {
		t.Errorf("titleId 0: %q", got.Titles[0].TitleID)
	}
	if !got.Titles[0].IsStreamable || got.Titles[0].ProductID != "9P20JCF7BV93" {
		t.Errorf("streamable/product: %+v", got.Titles[0])
	}
	if string(got.Titles[1].TitleID) != "12345" {
		t.Errorf("titleId 1: %q", got.Titles[1].TitleID)
	}
	e := titleToGameEntry(got.Titles[0])
	if e == nil {
		t.Fatal("titleToGameEntry")
	}
	if !e.XcloudAvailable || e.StoreProductID != "9P20JCF7BV93" || e.XcloudURL == "" {
		t.Fatalf("entry: %+v", e)
	}
	if want := "https://www.xbox.com/en-US/play/launch/enter-the-gungeon/9P20JCF7BV93"; e.XcloudURL != want {
		t.Errorf("xcloud URL: %q want %q", e.XcloudURL, want)
	}
	if got := xboxPlaySlug("Enter the Gungeon™"); got != "enter-the-gungeon" {
		t.Errorf("slug: %q", got)
	}
}
