package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"time"

	"cloud.google.com/go/storage"
)

// getLocationsFromStorage populates the output slice with the contents of the blob located at bucket/object.
func getLocationsFromStorage(bucket, object string) ([]location, error) {
	log.Printf("Downloading blob '%s'...", path.Join(bucket, object))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get new Storage client: %w", err)
	}

	defer func() {
		if errClose := client.Close(); errClose != nil {
			log.Print(errClose)
		}
	}()

	rc, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get Storage object %s: %w", path.Join(bucket, object), err)
	}

	defer func() {
		if errClose := rc.Close(); errClose != nil {
			log.Print(errClose)
		}
	}()

	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("could not read Storage object %s: %w", path.Join(bucket, object), err)
	}

	log.Printf("Blob '%s' downloaded", path.Join(bucket, object))

	locs := make([]location, 0)
	if err := json.Unmarshal(data, &locs); err != nil {
		return nil, fmt.Errorf("could not unmarshal location data: %w", err)
	}

	return locs, nil
}
