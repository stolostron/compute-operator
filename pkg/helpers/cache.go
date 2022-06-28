// Copyright Red Hat

package helpers

import (
	kcpcache "github.com/kcp-dev/apimachinery/pkg/cache"
	"github.com/kcp-dev/apimachinery/third_party/informers"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

var (
	NewCacheFunc = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		opts.NewInformerFunc = informers.NewSharedIndexInformer
		opts.Indexers = k8scache.Indexers{
			kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
			kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
		}
		return cache.New(config, opts)
	}
)
