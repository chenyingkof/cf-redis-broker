package backup

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/pivotal-cf/cf-redis-broker/backup/s3bucket"
	"github.com/pivotal-cf/cf-redis-broker/backupconfig"
	"github.com/pivotal-cf/cf-redis-broker/redis/client"
	"github.com/pivotal-cf/cf-redis-broker/redisconf"
	"github.com/pivotal-golang/lager"
)

type Backup struct {
	Config *backupconfig.Config
	Logger lager.Logger
}

// http://golang.org/pkg/time/#pkg-constants if you need to understand these crazy layouts
const timeFormat = "200601021504"

func (backup Backup) Create(configPath, instanceDataPath, instanceID, planName string) error {
	if err := backup.createSnapshot(configPath); err != nil {
		return err
	}

	timestamp := time.Now().Format(timeFormat)

	pathToRdbFile := path.Join(instanceDataPath, "dump.rdb")
	if !fileExists(pathToRdbFile) {
		backup.Logger.Info("dump.rdb not found, skipping instance backup", lager.Data{
			"Local file": pathToRdbFile,
		})
		return nil
	}

	bucket, err := backup.getOrCreateBucket()
	if err != nil {
		return err
	}
	return backup.uploadToS3(instanceID, planName, pathToRdbFile, timestamp, bucket)
}

func (backup Backup) getOrCreateBucket() (s3bucket.Bucket, error) {
	s3Client := s3bucket.NewClient(
		backup.Config.S3Configuration.EndpointUrl,
		backup.Config.S3Configuration.AccessKeyId,
		backup.Config.S3Configuration.SecretAccessKey,
	)

	return s3Client.GetOrCreate(backup.Config.S3Configuration.BucketName)
}

func (backup Backup) createSnapshot(instancePath string) error {
	client, err := backup.buildRedisClient(instancePath)
	if err != nil {
		return err
	}

	return client.CreateSnapshot(backup.Config.BGSaveTimeoutSeconds)
}

func (backup Backup) buildRedisClient(instancePath string) (*client.Client, error) {
	instanceConf, err := redisconf.Load(path.Join(instancePath, "redis.conf"))
	if err != nil {
		return nil, err
	}

	return client.Connect("localhost", instanceConf)
}

func (backup Backup) uploadToS3(instanceID, planName, pathToRdbFile string, timestamp string, bucket s3bucket.Bucket) error {
	rdbBytes, err := ioutil.ReadFile(pathToRdbFile)
	if err != nil {
		return err
	}

	remotePath := fmt.Sprintf("%s/%s_%s_%s_redis_backup.tgz", backup.Config.S3Configuration.Path, timestamp, instanceID, planName)

	backup.Logger.Info("Backing up instance", lager.Data{
		"Local file":  pathToRdbFile,
		"Remote file": remotePath,
	})

	return bucket.Upload(rdbBytes, remotePath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}
