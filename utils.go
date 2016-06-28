package main

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

func CalculateSha1(filePath string) (string, error) {
	// Initialize variable returnSha1String now in case an error has to be returned
	var returnSha1String string

	// Open the passed argument and check for any error
	file, err := os.Open(filePath)
	if err != nil {
		return returnSha1String, err
	}

	// Tell the program to call the following function when the current function returns
	defer file.Close()

	// Open a new hash interface to write to
	hash := sha1.New()

	//Copy the file in the hash interface and check for any error
	if _, err := io.Copy(hash, file); err != nil {
		return returnSha1String, err
	}

	// Get the 16 bytes hash
	hashInBytes := hash.Sum(nil) //[:16]

	// Convert the bytes to a string
	returnSha1String = strings.ToLower(hex.EncodeToString(hashInBytes))

	file.Close()
	return returnSha1String, nil
}
