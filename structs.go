package main

type BoxInfo struct {
	Id           uint64 `json:"id"`
	Url          string `json:"url"`
	Checksum     string `json:"checksum"`
	ChecksumType string `json:"checksum_type"`
}

type AvailableBoxes struct {
	Files []BoxInfo `json:"files"`
}
