/*
SPDX-License-Identifier: Apache-2.0
Copyright (C) 2022-2023 Intel Corporation
Copyright (c) 2022 Dell Inc, or its subsidiaries.
Copyright (C) 2022 Red Hat.
*/

// Package secureagent implements the secure agent
package secureagent

import (
	"bytes"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"time"

	"github.com/github/smimesign/ietf-cms/protocol"
	"github.com/opiproject/sztp/sztp-agent/pkg/dhcp"
)

const (
	// PRE nolint:var-naming
	PRE = "pre"
	// POST nolint:var-naming
	POST = "post"
)

// RunCommandDaemon runs the command in the background
func (a *Agent) RunCommandDaemon() error {
	for {
		err := a.performBootstrapSequence()
		if err != nil {
			log.Println("[ERROR] Failed to perform the bootstrap sequence: ", err.Error())
			log.Println("[INFO] Retrying in 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		}
		return nil
	}
}

func (a *Agent) performBootstrapSequence() error {
	var err error
	err = a.discoverBootstrapURLs()
	if err != nil {
		return err
	}

	urls := a.GetBootstrapURL()
	log.Println("[INFO] Performing the Bootstrap Sequence on each URL")
	for _, url := range urls {
		urlCopy := url
		err = a.doRequestBootstrapServerOnboardingInfo(&urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		err = a.doHandleBootstrapRedirect(&urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		err = a.downloadAndValidateImage(&urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		err = a.copyConfigurationFile(&urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		err = a.launchScriptsConfiguration(PRE, &urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		err = a.launchScriptsConfiguration(POST, &urlCopy)
		if err != nil {
			log.Println("[ERROR] ", err.Error())
			continue
		}
		_ = a.doReportProgress(ProgressTypeBootstrapComplete, "Bootstrap Complete", &urlCopy)
	}

	return nil
}

func (a *Agent) discoverBootstrapURLs() error {
	log.Println("[INFO] Discovering the Bootstrap URL")
	if a.InputBootstrapURL != "" {
		log.Println("[INFO] User gave us the Bootstrap URL: " + a.InputBootstrapURL)
		a.SetBootstrapURL([]string{a.InputBootstrapURL})
		log.Println("[INFO] Bootstrap URL retrieved successfully: ")
		log.Println(a.GetBootstrapURL())
		return nil
	}
	if a.DhcpLeaseFile != "" {
		log.Println("[INFO] User gave us the DHCP Lease File: " + a.DhcpLeaseFile)
		urls, err := dhcp.GetBootstrapURLsViaLeaseFile(a.DhcpLeaseFile, SZTP_REDIRECT_URL)
		if err != nil {
			return err
		}
		a.SetBootstrapURL(urls)
		log.Println("[INFO] Bootstrap URL retrieved successfully: ")
		log.Println(a.GetBootstrapURL())
		return nil
	}
	log.Println("[INFO] User gave us nothing, discover the Bootstrap URL from Network Manager via dbus")
	// TODO: fetch the Bootstrap URL from Network Manager via dbus in the future
	urls, err := dhcp.GetBootstrapURLsViaNetworkManager()
	if err != nil {
		return err
	}
	a.SetBootstrapURL(urls)
	log.Println("[INFO] Bootstrap URL retrieved successfully: ")
	log.Println(a.GetBootstrapURL())
	return nil
}

func (a *Agent) doHandleBootstrapRedirect(bootstrapURL *string) error {
	if reflect.ValueOf(a.BootstrapServerRedirectInfo).IsZero() {
		return nil
	}

	log.Println("[INFO] Go Re-direct instead of On-boarding, processing...")

	// TODO: BootstrapServer can be an array
	// TODO: do not ignore BootstrapServer[0].TrustAnchor
	addr := a.BootstrapServerRedirectInfo.IetfSztpConveyedInfoRedirectInformation.BootstrapServer[0].Address
	port := a.BootstrapServerRedirectInfo.IetfSztpConveyedInfoRedirectInformation.BootstrapServer[0].Port

	if addr == "" {
		return errors.New("invalid redirect address")
	}
	if port <= 0 {
		return errors.New("invalid port")
	}
	// Change URL to point to new redirect IP and PORT
	u, err := url.Parse(*bootstrapURL)
	if err != nil {
		return err
	}
	u.Host = fmt.Sprintf("%s:%d", addr, port)
	*bootstrapURL = u.String()

	// Request onboard info again (with new URL now)
	return a.doRequestBootstrapServerOnboardingInfo(bootstrapURL)
}

func (a *Agent) doRequestBootstrapServerOnboardingInfo(bootstrapURL *string) error {
	log.Println("[INFO] Starting the Request to get On-boarding Information.")
	res, err := a.doTLSRequest(a.GetInputJSONContent(), *bootstrapURL, false)
	if err != nil {
		log.Println("[ERROR] ", err.Error())
		return err
	}
	log.Println("[INFO] Response retrieved successfully")
	_ = a.doReportProgress(ProgressTypeBootstrapInitiated, "Bootstrap Initiated", bootstrapURL)
	crypto := res.IetfSztpBootstrapServerOutput.ConveyedInformation
	newVal, err := base64.StdEncoding.DecodeString(crypto)
	if err != nil {
		return err
	}
	ci, err := protocol.ParseContentInfo(newVal)
	if err != nil {
		return err
	}
	var data asn1.RawValue
	_, kerr := asn1.Unmarshal(ci.Content.Bytes, &data)
	if kerr != nil {
		return kerr
	}
	res.IetfSztpBootstrapServerOutput.ConveyedInformation = string(data.Bytes)
	decoderoi := json.NewDecoder(bytes.NewReader(data.Bytes))
	decoderoi.DisallowUnknownFields()
	var oi BootstrapServerOnboardingInfo
	erroi := decoderoi.Decode(&oi)
	if erroi == nil {
		a.BootstrapServerOnboardingInfo = oi
		log.Printf("[INFO] The BootstrapServerOnBoardingInfo object retrieved is: %v", a.BootstrapServerOnboardingInfo)
		return nil
	}
	decoderri := json.NewDecoder(bytes.NewReader(data.Bytes))
	decoderri.DisallowUnknownFields()
	var ri BootstrapServerRedirectInfo
	errri := decoderri.Decode(&ri)
	if errri == nil {
		a.BootstrapServerRedirectInfo = ri
		log.Printf("[INFO] The BootstrapServerRedirectInfo object retrieved is: %v", a.BootstrapServerRedirectInfo)
		return nil
	}
	return errri
}

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
		// TODO: maybe need to move sha calculatinos to a function in util.go
		switch a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.ImageVerification[i].HashAlgorithm {
		case "ietf-sztp-conveyed-info:sha-256":
			f, err := os.Open(ARTIFACTS_PATH + a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference + filepath.Base(item))
			if err != nil {
				log.Panic(err)
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Println("[ERROR] Error when closing:", err)
				}
			}()
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
			sum := fmt.Sprintf("%x", h.Sum(nil))
			original := strings.ReplaceAll(a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.BootImage.ImageVerification[i].HashValue, ":", "")
			log.Println("calculated: " + sum)
			log.Println("expected  : " + original)
			if sum != original {
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

func (a *Agent) copyConfigurationFile(bootstrapURL *string) error {
	log.Println("[INFO] Starting the Copy Configuration.")
	_ = a.doReportProgress(ProgressTypeConfigInitiated, "Configuration Initiated", bootstrapURL)
	// Copy the configuration file to the device
	file, err := os.Create(ARTIFACTS_PATH + a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference + "-config")
	if err != nil {
		log.Println("[ERROR] creating the configuration file", err.Error())
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Println("[ERROR] Error when closing:", err)
		}
	}()

	plainTest, _ := base64.StdEncoding.DecodeString(a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.Configuration)
	_, err = file.WriteString(string(plainTest))
	if err != nil {
		log.Println("[ERROR] writing the configuration file", err.Error())
		return err
	}
	// nolint:gosec
	err = os.Chmod(ARTIFACTS_PATH+a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference+"-config", 0744)
	if err != nil {
		log.Println("[ERROR] changing the configuration file permission", err.Error())
		return err
	}
	log.Println("[INFO] Configuration file copied successfully")
	_ = a.doReportProgress(ProgressTypeConfigComplete, "Configuration Complete", bootstrapURL)
	return nil
}

func (a *Agent) launchScriptsConfiguration(typeOf string, bootstrapURL *string) error {
	var script, scriptName string
	var reportStart, reportEnd ProgressType
	switch typeOf {
	case "post":
		script = a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.PostConfigurationScript
		scriptName = "post"
		reportStart = ProgressTypePostScriptInitiated
		reportEnd = ProgressTypePostScriptComplete
	default: // pre or default
		script = a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.PreConfigurationScript
		scriptName = "pre"
		reportStart = ProgressTypePreScriptInitiated
		reportEnd = ProgressTypePreScriptComplete
	}
	log.Println("[INFO] Starting the " + scriptName + "-configuration.")
	_ = a.doReportProgress(reportStart, "Report starting", bootstrapURL)
	// nolint:gosec
	file, err := os.Create(ARTIFACTS_PATH + a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference + scriptName + "configuration.sh")
	if err != nil {
		log.Println("[ERROR] creating the "+scriptName+"-configuration script", err.Error())
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Println("[ERROR] Error when closing:", err)
		}
	}()

	plainTest, _ := base64.StdEncoding.DecodeString(script)
	_, err = file.WriteString(string(plainTest))
	if err != nil {
		log.Println("[ERROR] writing the "+scriptName+"-configuration script", err.Error())
		return err
	}
	// nolint:gosec
	err = os.Chmod(ARTIFACTS_PATH+a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference+scriptName+"configuration.sh", 0755)
	if err != nil {
		log.Println("[ERROR] changing the "+scriptName+"-configuration script permission", err.Error())
		return err
	}
	log.Println("[INFO] " + scriptName + "-configuration script created successfully")
	cmd := exec.Command("/bin/sh", ARTIFACTS_PATH+a.BootstrapServerOnboardingInfo.IetfSztpConveyedInfoOnboardingInformation.InfoTimestampReference+scriptName+"configuration.sh") //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		log.Println("[ERROR] running the "+scriptName+"-configuration script", err.Error())
		return err
	}
	log.Println(string(out)) // remove it
	_ = a.doReportProgress(reportEnd, "Report end", bootstrapURL)
	log.Println("[INFO] " + scriptName + "-Configuration script executed successfully")
	return nil
}
