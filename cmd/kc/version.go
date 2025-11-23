package main

import (
	"context"
	"fmt"
	"io"
	"os"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
	componentversion "k8s.io/component-base/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type serverVersionGetter interface {
	ServerVersion() (*apimachineryversion.Info, error)
}

type versionInfo struct {
	kcVersion      string
	commit         string
	date           string
	clientGo       string
	serverVersion  *apimachineryversion.Info
}

func runVersionCommand(ctx context.Context) error {
	discoveryClient, err := buildDiscoveryClient(ctx)
	info, serverErr := collectVersionInfo(ctx, discoveryClient)
	printVersionInfo(os.Stdout, info)
	if serverErr != nil {
		fmt.Fprintf(os.Stderr, "Error fetching server version: %v\n", serverErr)
		return serverErr
	}
	return nil
}

func buildDiscoveryClient(ctx context.Context) (serverVersionGetter, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("creating client config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating discovery client: %w", err)
	}

	return &loggingDiscoveryClient{
		ctx:      ctx,
		host:     restConfig.Host,
		delegate: discoveryClient,
	}, nil
}

func collectVersionInfo(ctx context.Context, discoveryClient serverVersionGetter) (versionInfo, error) {
	info := versionInfo{
		kcVersion: version,
		commit:    commit,
		date:      date,
		clientGo:  componentversion.Get().GitVersion,
	}

	if discoveryClient == nil {
		return info, nil
	}

	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return info, err
	}

	info.serverVersion = serverVersion
	return info, nil
}

func printVersionInfo(out io.Writer, info versionInfo) {
	fmt.Fprintf(out, "kc Version: %s\n", info.kcVersion)
	fmt.Fprintf(out, "Client Version (client-go): %s\n", info.clientGo)
	if info.serverVersion != nil {
		fmt.Fprintf(out, "Server Version: %s\n", info.serverVersion.GitVersion)
	}
	fmt.Fprintf(out, "Commit: %s\n", info.commit)
	fmt.Fprintf(out, "Date: %s\n", info.date)
}

type loggingDiscoveryClient struct {
	ctx      context.Context
	host     string
	delegate discovery.ServerVersionInterface
}

func (l *loggingDiscoveryClient) ServerVersion() (*apimachineryversion.Info, error) {
	log := ctrllog.FromContext(l.ctx).WithName("version")
	log.Info("requesting server version", "host", l.host)
	return l.delegate.ServerVersion()
}
