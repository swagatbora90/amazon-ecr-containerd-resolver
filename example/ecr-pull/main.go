/*
 * Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"). You
 * may not use this file except in compliance with the License. A copy of
 * the License is located at
 *
 * 	http://aws.amazon.com/apache2.0/
 *
 * or in the "license" file accompanying this file. This file is
 * distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF
 * ANY KIND, either express or implied. See the License for the specific
 * language governing permissions and limitations under the License.
 */

package main

import (
	"context"
	"os"
	"strconv"
	"time"
	"fmt"

	"github.com/awslabs/amazon-ecr-containerd-resolver/ecr"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

const (
	// Default to no parallel layer downloading.
	defaultParallelism = 0
	// Default to no debug logging.
	defaultEnableDebug = 0
)

func main() {
	ctx := namespaces.NamespaceFromEnv(context.Background())

	if len(os.Args) < 2 {
		log.G(ctx).Fatal("Must provide image to pull as argument")
	} else if len(os.Args) > 2 {
		log.G(ctx).Fatal("Must provide only the image to pull")
	}

	ref := os.Args[1]

	parallelism := defaultParallelism
	parseEnvInt(ctx, "ECR_PULL_PARALLEL", &parallelism)

	enableDebug := defaultEnableDebug
	parseEnvInt(ctx, "ECR_PULL_DEBUG", &enableDebug)
	if enableDebug == 1 {
		log.L.Logger.SetLevel(logrus.TraceLevel)
	}

	address := "/run/containerd/containerd.sock"
	if newAddress := os.Getenv("CONTAINERD_ADDRESS"); newAddress != "" {
		address = newAddress
	}
	client, err := containerd.New(address)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("Failed to connect to containerd")
	}
	defer client.Close()

	ongoing := newJobs(ref)
	pctx, stopProgress := context.WithCancel(ctx)
	progress := make(chan struct{})
	go func() {
		showProgress(pctx, ongoing, client.ContentStore(), os.Stdout)
		close(progress)
	}()

	h := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
			ongoing.add(desc)
		}
		return nil, nil
	})
	
	//resolver, err := ecr.NewResolver(ecr.WithLayerDownloadParallelism(parallelism))
	//if err != nil {
	//	log.G(ctx).WithError(err).Fatal("Failed to create resolver")
	//}

	options := docker.ResolverOptions{}
	username := "AWS"
	secret := "eyJwYXlsb2FkIjoiZzBvZVp4YTY3bGc0SVpyRWdrd0xFQWFpNGNac2FGT0ExSitSZ0ZwWmtvKzJteW1wNm9rdjkyOGNaWUtRbnp3cGl5c1RwR2FnOWt4M3lCdUpDUlBmZjdtVEVWenZad1orSjRRZkw4b0JSZ1NQM0hRTCs3TCs5UlpsWGVqRjNkNyt6Z1FmOU1TOVlHQ0JrWTlIQlBYcVI1ankyZk9jeWgxeTREam9XQ1U2OWozY0NIMUo2QVR5UzZzYllwbzE0RjRFU1BsVkFudzN0Z1ZCMVExUUlhRmFFT2t5R3NvWitBdVlNbVNSNDV3TEV0ekJqK2NtcUlLcitmQ3ZqTFJjZzRiSVZsUFhuWGlWQmoxQnBoN0JzbnRhbVh1WGMzcVdGenZhQnlvSFlKV3o5ZjZ1ZnYxL0xYWGppTCtVY2g1Qm1ZTmNFTjB3U1hDL203elZMbG85RmJBNXNDZ2xaM3llbmtYOWkyeEJRd0I2ZFVIaWUvemIwbTd5SXF5WHBCYzVxSEw1MUdWRk5LeUc0WU12R1I2Y2tVSzlWRDUrUXNPZFdpb25VZ2ROakhwb1h5cithRGw1NDdON3hXaFZERTBtT3RaUDFXQ1RDVHFZempzUkNxUUNTUklSbGE0cGhyRlgzeDZITWlabkFCL014cmRWU2toQ3dVTWYwcjlZblR5cVZGakpiRVBqTklVdUVzV2UzM0RkbCtRZXZhNUtrZWxVcUJEYk9XU25OMEg4ZTFzTFBESVBrZWwxZEpnRmNXbWhsYVJ4aHJWaDdoOWoycmFaZjdXbWRZbVRKWThqaEJtSEhxZURIRVBNbFlDOUJwaWJRY0F3eEh1S1p5TnFTKzhqZjZsNmR3WFdZWFRxengvSzlJSWlzNEU4c3NMdDFxOWwrVEpwMDJZYnYvNjJDamoyVlZtVFVIdGllOFoyOEZUVjhuQW1KbFVzOWgrY1dKellGQjRwQWQ2TnlDNmxjU0FQdGdoMzVIRFM1OVlDc3FWZVZNRkZoNnBBZm1INHhNWVA4clIzOWZTbjBGaXFCMjFIeEF0bGwrenIyZTFkY3YzRnptSEQzekJ3VVIzU1IvdUhUR1VYK2ZUaGJMdlJzSjVFVno5Rk8xNjFJamw1WW1DSGE3S2E1aTBwODZHV2Z0RUdZcnA1ZHkzcWJZK3liSHhkRFk0bCs0clkzcTJOWkdyL0NRTDQ3Qmw4Y2ZsK01qR2tpME1Mdm5MMEwwY25sVEs0eFZzMHIvRThQOTZGczllVHR6MFB0U1JXdUFiQk9WUVZOZXQ4dUJ3S29yU1cybWV1RzRIaERCNXhTMkkxMU93NjQ2N2tHTXBUbzhhVWdCK1g2allwZWE2OWpqa3g5MzBBeFdodVhSY24wUUNGYmFJR0FlaW00VW9WQVJZVm1UM09saytCSXJpdFR6cWlVaGZ0clk2MExhVGNqQUMvOXNncjY1b1EvTkNsSmU5ZE8ybTdhdWkwVUMwOXEwdjUxRytFMWZiOTJ6MU1CbGtEK1Z3cHd4bEZuMWJqd1BHSDE3cWNCYW5mU0NqUWpvUXpIY3Q5TEQwYksrdS9hb2xiUUlrL1Zza09PQndqQUF6Z1V6cmNwWDdhWktnUStwTkoyQmNYaHdlM1hIbWtLYWVnRDFkcjc1V3k1cEVadFhCbHJDaWl4V3RMK0xEKzVHVFoyRHVrNmpaM1ZDN2hSTUsrRXpqQ1FvWWYrZklxdGYyRG5Jc3d4K3lKb2FFN0RIcFdGS1lWbzBZK2xRMTRRTzB2RW1yWDQxUWpRSjZTSUk0cVF6eitXMEhNNHZ2bFh6Ui9FVXh0UlFIYmIxK204VGFBM09RN2pBK1NpMUJmTUhLSlBCQmVSUVAxUDJQSkJOZWFQYmdYb3RWckI4bXdGL2dYYjEvUnlncjNjWU4wMm5mNnBueXNnMHJJbHcrMURvdVpjTmJtQmxESWF0blE2YmFWNEo3YnpXekEzMFpKNWw3MncyUGNjd1E2Szg1SFpGT0RBRld1MCtGNVZWNThFUHhoWWlhYnc0ZGpzcWVmTWkzM0M4NEhsQUpaQyswSjM1U2ZzSk12UlRYR3ZzMHZoS3R4WWZCRWlwaXRRQjdRSHY2d2h4a040d2xoRDM4cHUveFM4cEdnVDM4clBGc3ZwRnlDUzF1UXcrRTZXVGRnbC9uQytKcExQTDhzNHpkWmcxdnhpbHg0czdUcXhUZ3F6clpOUWovQTB4Uk9YVDJEMmlDcFh0TEh1VEttQ04yUUFMS1lWUFNhWGkzeVg4ZUoyb0JZVGFCWnpKSTRZYk9MNmVyR0hSRFpneUlMbzB3RFB3Z2wyY1FKVGJRYm9ERHV6UVFyWFVTWkxIS3VqRlAyYm9YZElNZHgzaERyTkN5encvMWpQNGlmWit0bW5HMDNWSHNRN0o3U3RpdVJJaHhYREUrdCtKTnBQTjF2eWZTYUg0cEtQS2tnb3AvSFBhV1NIRFJHQXZxMEo2UW1OYzBlRFhFN1IvT052eE96Q2ZadjdidnE4b0ZUNHkvT0QvZXdLRUVWVjFpMUpWYzRuVGtGSnU5N0poM0REWkNVOGI5Y0RUcmJyL3dreHJQc3BnPT0iLCJkYXRha2V5IjoiQVFFQkFIajZsYzRYSUp3LzdsbjBIYzAwRE1lazZHRXhIQ2JZNFJJcFRNQ0k1OEluVXdBQUFINHdmQVlKS29aSWh2Y05BUWNHb0c4d2JRSUJBREJvQmdrcWhraUc5dzBCQndFd0hnWUpZSVpJQVdVREJBRXVNQkVFRE56YmU2N1dBVHVtakVuUi9RSUJFSUE3U3BFaWUzcUptS01DL284VHdHUTdBK0NvREJmYmlZL0RIZUhTY0IvUThLTExhVER1V05pTDUwMzRBa2oyaEpSRThURi9vQXVCZnNneDd0Zz0iLCJ2ZXJzaW9uIjoiMiIsInR5cGUiOiJEQVRBX0tFWSIsImV4cGlyYXRpb24iOjE3MTkwNDExNjF9"
	hostOptions := config.HostOptions{}
	hostOptions.Credentials = func(host string) (string, string, error) {
                 //If host doesn't match...
                // Only one host
              return username, secret, nil
        }
	options.Hosts = config.ConfigureHosts(ctx, hostOptions)
	resolver := docker.NewResolver(options)

	log.G(ctx).WithField("ref", ref).Info("Pulling from Amazon ECR")
	img, err := client.Pull(ctx, ref,
		containerd.WithResolver(resolver),
		containerd.WithImageHandler(h),
		containerd.WithSchema1Conversion,
		containerd.WithMaxConcurrentDownloads(3),
		containerd.WithPullUnpack)
	stopProgress()
	if err != nil {
		log.G(ctx).WithError(err).WithField("ref", ref).Fatal("Failed to pull")
	}
	<-progress
	log.G(ctx).WithField("img", img.Name()).Info("Pulled successfully!")
	if skipUnpack := os.Getenv("ECR_SKIP_UNPACK"); skipUnpack != "" {
		return
	}
	snapshotter := containerd.DefaultSnapshotter
	if newSnapshotter := os.Getenv("CONTAINERD_SNAPSHOTTER"); newSnapshotter != "" {
		snapshotter = newSnapshotter
	}
	
	start := time.Now()
	log.G(ctx).
		WithField("img", img.Name()).
		WithField("snapshotter", snapshotter).
		Info("unpacking...")
	err = img.Unpack(ctx, snapshotter)
	if err != nil {
		log.G(ctx).WithError(err).WithField("img", img.Name).Fatal("Failed to unpack")
	}
	fmt.Println("unpackTime", time.Since(start).Seconds())
}

func parseEnvInt(ctx context.Context, varname string, val *int) {
	if varval := os.Getenv(varname); varval != "" {
		parsed, err := strconv.Atoi(varval)
		if err != nil {
			log.G(ctx).WithError(err).Fatalf("Failed to parse %s", varname)
		}
		*val = parsed
	}
}
