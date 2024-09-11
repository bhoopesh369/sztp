/*
SPDX-License-Identifier: Apache-2.0
Copyright (C) 2022-2023 Intel Corporation
Copyright (c) 2022 Dell Inc, or its subsidiaries.
Copyright (C) 2022 Red Hat.
*/

package secureagent

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//nolint:funlen
func (a *Agent) downloadAndValidateImage(bootstrapURL *string) error {
	log.Printf("[INFO] Starting the Download Image: %v", a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.DownloadURI)
	_ = a.doReportProgress(ProgressTypeBootImageInitiated, "BootImage Initiated", bootstrapURL)
	// Download the image from DownloadURI and save it to a file
	a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference = fmt.Sprintf("%8d", time.Now().Unix())
	for i, item := range a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.DownloadURI {
		// TODO: maybe need to file download to a function in util.go
		log.Printf("[INFO] Downloading Image %v", item)
		// Create a empty file
		file, err := os.Create(ARTIFACTS_PATH + a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference + filepath.Base(item))
		if err != nil {
			return err
		}

		caCert, _ := os.ReadFile(a.GetBootstrapTrustAnchorCert())
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		cert, _ := tls.LoadX509KeyPair(a.GetDeviceEndEntityCert(), a.GetDevicePrivateKey())

		check := http.Client{
			CheckRedirect: func(r *http.Request, _ []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					//nolint:gosec
					InsecureSkipVerify: true, // TODO: remove skip verify
					RootCAs:            caCertPool,
					Certificates:       []tls.Certificate{cert},
				},
			},
		}

		response, err := check.Get(item)
		if err != nil {
			return err
		}

		sizeorigin, _ := strconv.Atoi(response.Header.Get("Content-Length"))
		downloadSize := int64(sizeorigin)
		log.Printf("[INFO] Downloading the image with size: %v", downloadSize)

		if response.StatusCode != 200 {
			return errors.New("received non 200 response code")
		}
		size, err := io.Copy(file, response.Body)
		if err != nil {
			return err
		}
		defer func() {
			if err := file.Close(); err != nil {
				log.Println("[ERROR] Error when closing:", err)
			}
		}()
		defer func() {
			if err := response.Body.Close(); err != nil {
				log.Println("[ERROR] Error when closing:", err)
			}
		}()

		log.Printf("[INFO] Downloaded file: %s with size: %d", ARTIFACTS_PATH+a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference+filepath.Base(item), size)
		log.Println("[INFO] Verify the file checksum: ", ARTIFACTS_PATH+a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference+filepath.Base(item))
		switch a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.ImageVerification[i].HashAlgorithm {
		case "ietf-sztp-conveyed-info:sha-256":
			filePath := ARTIFACTS_PATH + a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference + filepath.Base(item)
			checksum, err := calculateSHA256File(filePath)
			original := strings.ReplaceAll(a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.ImageVerification[i].HashValue, ":", "")
			if err != nil {
				log.Println("[ERROR] Could not calculate checksum", err)
			}
			log.Println("calculated: " + checksum)
			log.Println("expected  : " + original)
			if checksum != original {
				return errors.New("checksum mismatch")
			}
			log.Println("[INFO] Checksum verified successfully")
			_ = a.doReportProgress(ProgressTypeBootImageComplete, "BootImage Complete", bootstrapURL)
			return nil
		default:
			return errors.New("unsupported hash algorithm")
		}
	}
	return nil
}
