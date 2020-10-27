package kustomize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/quay/config-tool/pkg/lib/fieldgroups/database"
	"github.com/quay/config-tool/pkg/lib/fieldgroups/distributedstorage"
	"github.com/quay/config-tool/pkg/lib/fieldgroups/redis"
	"github.com/quay/config-tool/pkg/lib/fieldgroups/repomirror"
	"github.com/quay/config-tool/pkg/lib/fieldgroups/securityscanner"
	"github.com/quay/config-tool/pkg/lib/shared"
	v1 "github.com/quay/quay-operator/apis/quay/v1"
)

func quayRegistry(name string) *v1.QuayRegistry {
	return &v1.QuayRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				v1.SupportsObjectStorageAnnotation: "true",
				v1.StorageHostnameAnnotation:       "s3.noobaa.svc",
				v1.StorageBucketNameAnnotation:     "quay-datastore",
				v1.StorageAccessKeyAnnotation:      "abc123",
				v1.StorageSecretKeyAnnotation:      "super-secret",
			},
		},
		Spec: v1.QuayRegistrySpec{
			Components: []v1.Component{
				{Kind: "postgres", Managed: true},
				{Kind: "clair", Managed: true},
				{Kind: "redis", Managed: true},
				{Kind: "objectstorage", Managed: true},
			},
		},
	}
}

var fieldGroupForTests = []struct {
	name      string
	component string
	quay      *v1.QuayRegistry
	expected  shared.FieldGroup
}{
	{
		"clair",
		"clair",
		quayRegistry("test"),
		&securityscanner.SecurityScannerFieldGroup{
			FeatureSecurityScanner:              true,
			SecurityScannerIndexingInterval:     30,
			SecurityScannerNotifications:        true,
			SecurityScannerV4Endpoint:           "http://test-clair-app:80",
			SecurityScannerV4NamespaceWhitelist: []string{"admin"},
			SecurityScannerV4PSK:                "abc123",
		},
	},
	{
		"redis",
		"redis",
		quayRegistry("test"),
		&redis.RedisFieldGroup{
			BuildlogsRedis: &redis.BuildlogsRedisStruct{
				Host: "test-quay-redis",
				Port: 6379,
			},
			UserEventsRedis: &redis.UserEventsRedisStruct{
				Host: "test-quay-redis",
				Port: 6379,
			},
		},
	},
	{
		"postgres",
		"postgres",
		quayRegistry("test"),
		&database.DatabaseFieldGroup{
			DbUri: "postgresql://test-quay-database:postgres@test-quay-database:5432/test-quay-database",
			DbConnectionArgs: &database.DbConnectionArgsStruct{
				Autorollback: true,
				Threadlocals: true,
			},
		},
	},
	{
		"objectstorage",
		"objectstorage",
		quayRegistry("test"),
		&distributedstorage.DistributedStorageFieldGroup{
			FeatureProxyStorage:                true,
			DistributedStoragePreference:       []string{"local_us"},
			DistributedStorageDefaultLocations: []string{"local_us"},
			DistributedStorageConfig: map[string]*distributedstorage.DistributedStorageDefinition{
				"local_us": {
					Name: "RHOCSStorage",
					Args: &shared.DistributedStorageArgs{
						AccessKey:   "abc123",
						BucketName:  "quay-datastore",
						Hostname:    "s3.noobaa.svc",
						IsSecure:    true,
						Port:        443,
						SecretKey:   "super-secret",
						StoragePath: "/datastorage/registry",
					},
				},
			},
		},
	},
	{
		"horizontalpodautoscaler",
		"horizontalpodautoscaler",
		quayRegistry("test"),
		nil,
	},
	{
		"mirror",
		"mirror",
		quayRegistry("test"),
		&repomirror.RepoMirrorFieldGroup{
			FeatureRepoMirror:   true,
			RepoMirrorInterval:  30,
			RepoMirrorTlsVerify: true,
		},
	},
}

func TestFieldGroupFor(t *testing.T) {
	assert := assert.New(t)

	for _, test := range fieldGroupForTests {
		fieldGroup, err := FieldGroupFor(test.component, test.quay)
		assert.Nil(err, test.name)

		// TODO(alecmerdler): Make this more generic for other randomly-generated fields.
		if test.name == "clair" {
			secscanFieldGroup := fieldGroup.(*securityscanner.SecurityScannerFieldGroup)

			assert.True(len(secscanFieldGroup.SecurityScannerV4PSK) > 0, test.name)
			secscanFieldGroup.SecurityScannerV4PSK = "abc123"
		}

		expected, err := yaml.Marshal(test.expected)
		assert.Nil(err, test.name)
		configFields, err := yaml.Marshal(fieldGroup)
		assert.Nil(err, test.name)

		assert.Equal(string(expected), string(configFields), test.name)
	}
}

var containsComponentConfigTests = []struct {
	name          string
	component     string
	rawConfig     string
	expected      bool
	expectedError error
}{
	{
		"ClairContains",
		"clair",
		`FEATURE_SECURITY_SCANNER: true`,
		true,
		nil,
	},
	{
		"ClairDoesNotContain",
		"clair",
		``,
		false,
		nil,
	},
	{
		"PostgresContains",
		"postgres",
		`DB_URI: postgresql://test-quay-database:postgres@test-quay-database:5432/test-quay-database`,
		true,
		nil,
	},
	{
		"PostgresDoesNotContain",
		"postgres",
		``,
		false,
		nil,
	},
	{
		"RedisContains",
		"redis",
		`BUILDLOGS_REDIS:
  host: test-quay-redis
`,
		true,
		nil,
	},
	{
		"RedisDoesNotContain",
		"redis",
		``,
		false,
		nil,
	},
	{
		"ObjectStorageContains",
		"objectstorage",
		`DISTRIBUTED_STORAGE_PREFERENCE: 
  - local_us
`,
		true,
		nil,
	},
	{
		"ObjectStorageDoesNotContain",
		"objectstorage",
		``,
		false,
		nil,
	},
	{
		"MirrorContains",
		"mirror",
		`FEATURE_REPO_MIRROR: true`,
		true,
		nil,
	},
	{
		"MirrorDeosNotContain",
		"mirror",
		``,
		false,
		nil,
	},
	{
		"RouteContains",
		"route",
		`PREFERRED_URL_SCHEME: http`,
		true,
		nil,
	},
	{
		"RouteContainsServerHostname",
		"route",
		`SERVER_HOSTNAME: registry.skynet.com`,
		false,
		nil,
	},
	{
		"RouteDoesNotContain",
		"route",
		``,
		false,
		nil,
	},
	{
		"HorizontalPodAutoscalerDoesNotContain",
		"horizontalpodautoscaler",
		``,
		false,
		nil,
	},
}

func TestContainsComponentConfig(t *testing.T) {
	assert := assert.New(t)

	for _, test := range containsComponentConfigTests {
		var fullConfig map[string]interface{}
		err := yaml.Unmarshal([]byte(test.rawConfig), &fullConfig)
		assert.Nil(err, test.name)

		contains, err := ContainsComponentConfig(fullConfig, test.component)

		if test.expectedError != nil {
			assert.NotNil(err, test.name)
		} else {
			assert.Nil(err, test.name)
			assert.Equal(test.expected, contains, test.name)
		}
	}
}
