/*
Copyright the Velero contributors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"

	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

var (
	uuidgen uuid.UUID
)

// Test backup and restore of Kibishi using restic
var _ = Describe("[Restic] Velero tests on cluster using the plugin provider for object storage and Restic for volume backups", backup_restore_with_restic)

var _ = Describe("[Snapshot] Velero tests on cluster using the plugin provider for object storage and snapshots for volume backups", backup_restore_with_snapshots)

func backup_restore_with_snapshots() {
	backup_restore_test(true)
}

func backup_restore_with_restic() {
	backup_restore_test(false)
}

func backup_restore_test(useVolumeSnapshots bool) {
	var (
		client           *kubernetes.Clientset
		extensionsClient *apiextensionsclientset.Clientset
		backupName       string
		restoreName      string
	)

	BeforeEach(func() {
		if useVolumeSnapshots && cloudProvider == "kind" {
			Skip("Volume snapshots not supported on kind")
		}
		var err error
		flag.Parse()
		uuidgen, err = uuid.NewRandom()
		Expect(err).To(Succeed())
		if installVelero {
			Expect(veleroInstall(context.Background(), veleroImage, veleroNamespace, cloudProvider, objectStoreProvider, useVolumeSnapshots,
				cloudCredentialsFile, bslBucket, bslPrefix, bslConfig, vslConfig, "")).To(Succeed())

		}
		client, extensionsClient, err = kube.GetClusterClient()
		Expect(err).To(Succeed(), "Failed to instantiate cluster client")
	})

	AfterEach(func() {
		if installVelero {
			timeoutCTX, _ := context.WithTimeout(context.Background(), time.Minute)
			err := veleroUninstall(timeoutCTX, client, extensionsClient, veleroNamespace)
			Expect(err).To(Succeed())
		}

	})

	When("kibishii is the sample workload", func() {
		It("should be successfully backed up and restored to the default BackupStorageLocation", func() {
			backupName = "backup-" + uuidgen.String()
			restoreName = "restore-" + uuidgen.String()
			// Even though we are using Velero's CloudProvider plugin for object storage, the kubernetes cluster is running on
			// KinD. So use the kind installation for Kibishii.
			Expect(runKibishiiTests(client, cloudProvider, veleroCLI, veleroNamespace, backupName, restoreName, "", useVolumeSnapshots)).To(Succeed(),
				"Failed to successfully backup and restore Kibishii namespace")
		})

		It("should successfully back up and restore to an additional BackupStorageLocation with unique credentials", func() {
			if additionalBSLProvider == "" {
				Skip("no additional BSL provider given, not running multiple BackupStorageLocation with unique credentials tests")
			}

			if additionalBSLBucket == "" {
				Skip("no additional BSL bucket given, not running multiple BackupStorageLocation with unique credentials tests")
			}

			if additionalBSLCredentials == "" {
				Skip("no additional BSL credentials given, not running multiple BackupStorageLocation with unique credentials tests")
			}

			Expect(veleroAddPluginsForProvider(context.TODO(), veleroCLI, veleroNamespace, additionalBSLProvider)).To(Succeed())

			// Create Secret for additional BSL
			secretName := fmt.Sprintf("bsl-credentials-%s", uuidgen)
			secretKey := fmt.Sprintf("creds-%s", additionalBSLProvider)
			files := map[string]string{
				secretKey: additionalBSLCredentials,
			}

			Expect(createSecretFromFiles(context.TODO(), client, veleroNamespace, secretName, files)).To(Succeed())

			// Create additional BSL using credential
			additionalBsl := fmt.Sprintf("bsl-%s", uuidgen)
			Expect(veleroCreateBackupLocation(context.TODO(),
				veleroCLI,
				veleroNamespace,
				additionalBsl,
				additionalBSLProvider,
				additionalBSLBucket,
				additionalBSLPrefix,
				additionalBSLConfig,
				secretName,
				secretKey,
			)).To(Succeed())

			bsls := []string{"default", additionalBsl}

			for _, bsl := range bsls {
				backupName = fmt.Sprintf("backup-%s-%s", bsl, uuidgen)
				restoreName = fmt.Sprintf("restore-%s-%s", bsl, uuidgen)

				Expect(runKibishiiTests(client, cloudProvider, veleroCLI, veleroNamespace, backupName, restoreName, bsl, useVolumeSnapshots)).To(Succeed(),
					"Failed to successfully backup and restore Kibishii namespace using BSL %s", bsl)
			}
		})
	})
}
