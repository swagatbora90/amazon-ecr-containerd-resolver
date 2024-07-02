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
	//"github.com/containerd/containerd/remotes/docker"
	//"github.com/containerd/containerd/remotes/docker/config"
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
	
	resolver, err := ecr.NewResolver(ecr.WithLayerDownloadParallelism(parallelism))
	if err != nil {
		log.G(ctx).WithError(err).Fatal("Failed to create resolver")
	}

	//options := docker.ResolverOptions{}
	//username := "AWS"
	//secret := "eyJwYXlsb2FkIjoiYWxRd0JaaTVQQ2toZURnb1BUaFZyRzZOZjJvUitGL24zWCttYjNnZXZQRGZsQ1R2aStGeUZUc2lpRmNVNHJudTRaK2R0clJqenVIcHlIQjNXMzd4WThsTDVqajM2VjF3TnFxVFFITnpNcURWYzB5UlZvVVExRzFsUVhtcnEyTHl1WEJUYUt4eGt2SzhqY08wRWxuN2lEbWkyQjY2dEpTbnlSbXRKMHYyWHNiL1p0YmxDeVpEMVRoNjJYUGVIK0FibmNRTjc4ZUN2b2lYUUJTOU4zWXphcjNyUmNudi9UZ3JzcFE4TC84VG54MWlmQ0FiRStKNmhtYjBJYS9VQXk5SmdNVU9FWFBhb2s3VllKTldJNkZHNUZ0WkhCQjhaMW8vNXBZRkdaUW9UOUVMZTl3SWtYMUNxYkFjM2N0VWhNcWN6OVNLWms4NW5kUzR0V3d5RFdHOUE0Qmh1Z2dEaVp2SDFLVTJQSkh2ZG9wR1MzZXo0UmJBQ09sbTVSakFkQy9kc05RSmVhQmx2Tll4a25Mekg5ZHJQdWl0TnEwMTRKNUpQaDNQVW5IdHpBM0trcWdhQW1LZmpoeDZlM25zdmVTRUhUcmNqeVFJMzlvSkxsa3dzVmFodG00VkNNSVV0RHhURFRXekh6WTBiTDNUV0dMK3BXNGxPNW42Z1VPMkYzRWtSNUYxbzh1YVZkWDFaYkRKMnpSR29LNHEyUWdyZXVBcmJmSE5qNFZuWkYxR1NNQm8wSjNINFhYVDh2M2FvMkRXQzk4a0dlR0Z6OWMzVGJTUXk0YVhjeVh6aVBaNTFoSUljL3JheHN3VGhHVjZaMGtZOWgydzhZa3EwQXFRUS9waTZTcVc4TWQyb3E1ZnBydXlhbU96ZTdVZ0Fta3FPdUxlTExXQWphVUd1TmhCank0VU5GbkxPcTlpUUl2SzliaktLTDRpNFV0ZmI4eDBNUUJ6K2VIVEUwNzJSWUpiVGNMeC82bFptdWdlbUt2K0FDWGhQbzVGZjgrQU42a0lWWmh0ODNCbWltL1hyODIrbS9BMnh0NnB6cmFHcFhQc0RocTVvVlRCYzhuT0MycjNwUlhyY0ZSQzRwRGVzQXJUeHh2NVQzUllnTjdIWmlyWTlCZGNHb1pENE5OUjJqeDJZNDJrdUxRZGJTY3BBR00wQjh6dEdXZlVqYjgzTEtQR0NON2F6OURFeG5iY2RqOHVYeDVjeUlJbDUwaE9FRkZvMlNUVFdQcXdMWDV5bHVjaG1mN1gyek5YbkR3a2ExZzVxcW9XS2FiNWh3SThNT1JYUnljZHVuYVRXWFV4SnVTVnl5c0lZSjBhdjJWM28vaSt1WjJjZjJNVVhCYS9qNjM0dXZBMmdzNWJhbnl6N2huNW5JTlpsSERyV055UTJya29ncExZNlhLeDJvYTRwTmZsUVA2Wmp3YjJlck9haUJTZVo4aDZ1cW41SVVDdG1hei9hM05mbksxOE12L2JSOHRUVFowdDBFSDhzNCttdVQ2aHhSYTNRRis3dFN6a056M2pZbnRRVkwycStHVEJUZG5pZFVVYzF2dkRlRGxYU1ArZmp2ZUVCL2FtN1poNVY4UmZUQ3lTaGN0cFVtL0NFbXlVOXlQdE95RDliRURFVFVLLzhYbW1hZyt6T1Y0UmwrTHdQY1ZmNWsrQm1KSEg2ZGt6TWxiNWF3eC9LSHllZktFVGNpd0xPVXQrZmphVHhZMld4ejJFbGVReERuUjZnMUpKanpnbzBJOEF6MmxSMjcxZWZFa0ZWblc2Tzg1TVhwRGRHbUFHZkRZWmhhKzZsT2o4V0ZEZWZTYzgvekhBSGVycDlNbWdBNFEwM0lLR2N4dTlGaXBiMHVZOG52bndrYm1HYVgyVWFWRU9kTkRPbklJeHk4SS9zZGJHRHpJK3lhQWdBN2tFYkFLaEJ6TURKMFpHeDNCMm1Md2g3eEc5Yys2am9abUxoQlFHQzhXWXg5cnlQTS9oL0UxODVRam5MeVg3Nk1BUGY4T0MyQmVrQnBmb1RPdlQwWkNZUlhpc3pwenFVSmZ4dGk0L1k1K3JsbmJrZGRwZjVlU0RudWVLUGdmMUZ2dkgxTFRwYnhIaHZzdmhZN0lyamN4Sm40ajR5MmJvTzZzZjdCV3VzLzVxM05jSHROYVIxUmcveHU0Q2VnRjRRR0hFdWtlZHNSbEZ4QWRYYldzZ0p6aVRJaDNSQUgyY0d5dW9oVGJITnlBQm5vNzliU1F6N2hyQWplSzlpUFFhb2Z1R0pCRS9tUlh6VjUyOHVzemhsQzVyVXJ3aGIwQVJWU0NSWG5tOFFON0dqcDdrNjYrWS9yekwwUXo2YVVoL0x3akVJSmI3cFA2aStzV2ErMmRNd1dJc2hTYmtMdVdOUXlXMWFGTFhFbnZaTkh4cDMxS0JrbndDbk1GNXVTQkdPQVRDWVNTbjlHT3crWWJVWFR2T1lWcnd3OW55dE95NUwxR0lyQTZqMi9ZSk1oTEt6ekR3bWZBR0lHSmxYbGNlTHllckNrcmQ2cWp1WlFQYzV3QkRqNGJOOEFzUHhxVnZqLzlQV3ZNcXQxaGNORGNLbFdSeUhRPT0iLCJkYXRha2V5IjoiQVFFQkFIajZsYzRYSUp3LzdsbjBIYzAwRE1lazZHRXhIQ2JZNFJJcFRNQ0k1OEluVXdBQUFINHdmQVlKS29aSWh2Y05BUWNHb0c4d2JRSUJBREJvQmdrcWhraUc5dzBCQndFd0hnWUpZSVpJQVdVREJBRXVNQkVFRE9ITjJ2RU5UMitkbndXcFR3SUJFSUE3SlI0SE9LNmlNSmpFSVUrd21FclBMMXJDSHBBeTg1UDZoLzNGbFRFWHd1N2c5aGtVbFk4MElZUk9SQWtHUlNLUzNpcGdybC96V2lrNGdwRT0iLCJ2ZXJzaW9uIjoiMiIsInR5cGUiOiJEQVRBX0tFWSIsImV4cGlyYXRpb24iOjE3MTkyMTg2MjV9"
	//hostOptions := config.HostOptions{}
	//hostOptions.Credentials = func(host string) (string, string, error) {
                 //If host doesn't match...
                // Only one host
          //    return username, secret, nil
        //}
	//options.Hosts = config.ConfigureHosts(ctx, hostOptions)
	//resolver := docker.NewResolver(options)

	log.G(ctx).WithField("ref", ref).Info("Pulling from Amazon ECR")
	img, err := client.Pull(ctx, ref,
		containerd.WithResolver(resolver),
		containerd.WithImageHandler(h),
		containerd.WithSchema1Conversion)
		//containerd.WithMaxConcurrentDownloads(3),
		//containerd.WithPullUnpack)
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
