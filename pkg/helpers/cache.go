// Copyright Red Hat

package helpers

import (
	kcpcache "github.com/kcp-dev/apimachinery/pkg/cache"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
)

var (
	NewCacheFunc = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		opts.KeyFunction = kcpcache.ClusterAwareKeyFunc
		opts.Indexers = k8scache.Indexers{
			kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
			kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
		}
		return cache.New(config, opts)
	}

	NewClusterAwareCacheFunc = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		opts.KeyFunction = kcpcache.ClusterAwareKeyFunc
		opts.Indexers = k8scache.Indexers{
			kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
			kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
		}
		return kcp.NewClusterAwareCache(config, opts)
	}
)
