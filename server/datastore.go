package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/golang/glog"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/client"
	"github.com/lukegb/snowstorm/ngdp/encoding"
	"github.com/lukegb/snowstorm/ngdp/mndx"
	"github.com/pkg/errors"
)

type DatastoreTracked struct {
	Region  ngdp.Region
	Program ngdp.ProgramCode
}

type datastore struct {
	llc *client.LowLevelClient

	// Guards all fields below.
	l sync.RWMutex

	tracking []DatastoreTracked

	cdnInfos     map[ngdp.ProgramCode]map[ngdp.Region]*ngdp.CDNInfo
	versionInfos map[ngdp.ProgramCode]map[ngdp.Region]*ngdp.VersionInfo

	// The below are indexed on their own CDNHashes.
	buildConfigs map[ngdp.CDNHash]*ngdp.BuildConfig
	cdnConfigs   map[ngdp.CDNHash]*ngdp.CDNConfig

	// The below are indexed on BuildConfig CDNHashes.
	encodingMappers map[ngdp.CDNHash]*encoding.Mapper
	filenameMappers map[ngdp.CDNHash]ngdp.FilenameMapper

	// The below is indexed on a CDNConfig CDNHash.
	archiveMappers map[ngdp.CDNHash]*client.ArchiveMapper
}

func newDatastore(llc *client.LowLevelClient) *datastore {
	return &datastore{
		llc:          llc,
		cdnInfos:     make(map[ngdp.ProgramCode]map[ngdp.Region]*ngdp.CDNInfo),
		versionInfos: make(map[ngdp.ProgramCode]map[ngdp.Region]*ngdp.VersionInfo),

		buildConfigs:    make(map[ngdp.CDNHash]*ngdp.BuildConfig),
		cdnConfigs:      make(map[ngdp.CDNHash]*ngdp.CDNConfig),
		encodingMappers: make(map[ngdp.CDNHash]*encoding.Mapper),
		filenameMappers: make(map[ngdp.CDNHash]ngdp.FilenameMapper),
		archiveMappers:  make(map[ngdp.CDNHash]*client.ArchiveMapper),
	}
}

func (d *datastore) Client(region ngdp.Region, program ngdp.ProgramCode) (*client.Client, error) {
	d.l.RLock()
	defer d.l.RUnlock()

	cdnInfo, ok := d.cdnInfos[program][region]
	if !ok {
		return nil, fmt.Errorf("CDNInfo missing for %q/%q", program, region)
	}

	versionInfo := d.versionInfos[program][region]
	if !ok {
		return nil, fmt.Errorf("VersionInfo missing for %q/%q", program, region)
	}

	buildConfig, ok := d.buildConfigs[versionInfo.BuildConfig]
	if !ok {
		return nil, fmt.Errorf("BuildConfig missing for %q/%q @ %032x", program, region, versionInfo.BuildConfig)
	}

	cdnConfig, ok := d.cdnConfigs[versionInfo.CDNConfig]
	if !ok {
		return nil, fmt.Errorf("CDNConfig missing for %q/%q @ %032x", program, region, versionInfo.CDNConfig)
	}

	encodingMapper, ok := d.encodingMappers[versionInfo.BuildConfig]
	if !ok {
		return nil, fmt.Errorf("EncodingMapper missing for %q/%q @ %032x", program, region, versionInfo.BuildConfig)
	}

	filenameMapper, ok := d.filenameMappers[versionInfo.BuildConfig]
	if !ok {
		return nil, fmt.Errorf("FilenameMapper missing for %q/%q @ %032x", program, region, versionInfo.BuildConfig)
	}

	archiveMapper, ok := d.archiveMappers[versionInfo.CDNConfig]
	if !ok {
		return nil, fmt.Errorf("ArchiveMapper missing for %q/%q @ %032x", program, region, versionInfo.CDNConfig)
	}

	return &client.Client{
		LowLevelClient: d.llc,

		CDNInfo:     cdnInfo,
		VersionInfo: versionInfo,

		BuildConfig: buildConfig,
		CDNConfig:   cdnConfig,

		ArchiveMapper:  archiveMapper,
		EncodingMapper: encodingMapper,
		FilenameMapper: filenameMapper,
	}, nil
}

// Update runs a single iteration of datastore's update loop, blocking until it is complete.
func (d *datastore) Update(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.l.RLock()
	tracking := make([]DatastoreTracked, len(d.tracking))
	copy(tracking, d.tracking)
	d.l.RUnlock()

	var err error
	for _, t := range tracking {
		err = d.update(ctx, t.Region, t.Program)
		if err != nil {
			glog.Errorf("Error updating %q/%q: %v", t.Program, t.Region, err)
		}
	}

	glog.Info("Looking for no-longer-referenced entities")
	usedBuildConfigs := make(map[ngdp.CDNHash]bool)
	usedCDNConfigs := make(map[ngdp.CDNHash]bool)
	d.l.Lock()
	for _, rs := range d.versionInfos {
		for _, version := range rs {
			usedBuildConfigs[version.BuildConfig] = true
			usedCDNConfigs[version.CDNConfig] = true
		}
	}
	var toDelete []ngdp.CDNHash

	toDelete = nil
	for h, _ := range d.buildConfigs {
		if !usedBuildConfigs[h] {
			toDelete = append(toDelete, h)
		}
	}
	for _, e := range toDelete {
		delete(d.buildConfigs, e)
	}
	if len(toDelete) > 0 {
		glog.Infof("Deleted %d build configs", len(toDelete))
	}

	toDelete = nil
	for h, _ := range d.cdnConfigs {
		if !usedCDNConfigs[h] {
			toDelete = append(toDelete, h)
		}
	}
	for _, e := range toDelete {
		delete(d.cdnConfigs, e)
	}
	if len(toDelete) > 0 {
		glog.Infof("Deleted %d CDN configs", len(toDelete))
	}

	toDelete = nil
	for h, _ := range d.encodingMappers {
		if !usedBuildConfigs[h] {
			toDelete = append(toDelete, h)
		}
	}
	for _, e := range toDelete {
		delete(d.encodingMappers, e)
	}
	if len(toDelete) > 0 {
		glog.Infof("Deleted %d encoding mappers", len(toDelete))
	}

	toDelete = nil
	for h, _ := range d.filenameMappers {
		if !usedBuildConfigs[h] {
			toDelete = append(toDelete, h)
		}
	}
	for _, e := range toDelete {
		delete(d.filenameMappers, e)
	}
	if len(toDelete) > 0 {
		glog.Infof("Deleted %d filename mappers", len(toDelete))
	}

	toDelete = nil
	for h, _ := range d.archiveMappers {
		if !usedCDNConfigs[h] {
			toDelete = append(toDelete, h)
		}
	}
	for _, e := range toDelete {
		delete(d.archiveMappers, e)
	}
	if len(toDelete) > 0 {
		glog.Infof("Deleted %d archive mappers", len(toDelete))
	}

	d.l.Unlock()

	glog.Info("Collecting garbage")
	runtime.GC()

	return err
}

// update updates a single region/program pair.
func (d *datastore) update(ctx context.Context, region ngdp.Region, program ngdp.ProgramCode) error {
	glog.Infof("Updating %q/%q", program, region)

	cdn, version, err := d.llc.Info(ctx, program, region)
	if err != nil {
		return errors.Wrap(err, "retrieving info")
	}

	d.l.RLock()
	oldVersion, haveOldVersion := d.versionInfos[program][region]
	buildConfig, haveBuildConfig := d.buildConfigs[version.BuildConfig]
	cdnConfig, haveCDNConfig := d.cdnConfigs[version.CDNConfig]
	d.l.RUnlock()

	if haveOldVersion {
		if oldVersion.VersionsName != version.VersionsName {
			glog.Infof("%q/%q: version string changed from %v to %v", program, region, oldVersion.VersionsName, version.VersionsName)
		}
		if oldVersion.BuildID != version.BuildID {
			glog.Infof("%q/%q: build ID changed from %v to %v", program, region, oldVersion.BuildID, version.BuildID)
		}
		if !oldVersion.BuildConfig.Equal(version.BuildConfig) {
			glog.Infof("%q/%q: build config changed from %032x to %032x", program, region, oldVersion.BuildConfig, version.BuildConfig)
		}
	}

	if !haveBuildConfig || !haveCDNConfig {
		glog.Infof("%q/%q: retrieving build config %032x", program, region, version.BuildConfig)
		glog.Infof("%q/%q: retrieving CDN config %032x", program, region, version.CDNConfig)

		cdnConfigS, buildConfigS, err := d.llc.Configs(ctx, cdn, version)
		if err != nil {
			return errors.Wrap(err, "retrieving configs")
		}

		buildConfig = &buildConfigS
		cdnConfig = &cdnConfigS

		d.l.Lock()
		d.buildConfigs[version.BuildConfig] = buildConfig
		d.cdnConfigs[version.CDNConfig] = cdnConfig
		d.l.Unlock()
	}

	d.l.RLock()
	encodingMapper, haveEncodingMapper := d.encodingMappers[version.BuildConfig]
	archiveMapper, haveArchiveMapper := d.archiveMappers[version.CDNConfig]
	d.l.RUnlock()

	if !haveEncodingMapper || !haveArchiveMapper {
		encodingMapper, archiveMapper, err = d.llc.Mappers(ctx, cdn, *cdnConfig, *buildConfig)
		if err != nil {
			return errors.Wrap(err, "retrieving mappers")
		}

		d.l.Lock()
		d.encodingMappers[version.BuildConfig] = encodingMapper
		d.archiveMappers[version.CDNConfig] = archiveMapper
		d.l.Unlock()
	}

	d.l.RLock()
	_, haveFilenameMapper := d.filenameMappers[version.BuildConfig]
	d.l.RUnlock()

	if !haveFilenameMapper {
		glog.Info("Building filename map")
		rootCDNHash, err := encodingMapper.ToCDNHash(buildConfig.Root)
		if err != nil {
			return errors.Wrap(err, "mapping root file hash to CDN hash")
		}

		root, err := d.llc.Fetch(ctx, cdn, rootCDNHash)
		if err != nil {
			return errors.Wrap(err, "fetching root file")
		}
		defer root.Close()

		mapper, err := mndx.Parse(root)
		if err != nil {
			return errors.Wrap(err, "parsing filename map")
		}

		tree, err := mndx.ToTree(mapper)
		if err != nil {
			return errors.Wrap(err, "treeifying filename map")
		}

		d.l.Lock()
		d.filenameMappers[version.BuildConfig] = tree
		d.l.Unlock()
	}

	d.l.Lock()
	d.cdnInfos[program][region] = &cdn
	d.versionInfos[program][region] = &version
	d.l.Unlock()

	return nil
}

func (d *datastore) Track(region ngdp.Region, program ngdp.ProgramCode) {
	d.l.Lock()
	defer d.l.Unlock()

	if _, ok := d.cdnInfos[program]; !ok {
		d.cdnInfos[program] = make(map[ngdp.Region]*ngdp.CDNInfo)
	}
	if _, ok := d.versionInfos[program]; !ok {
		d.versionInfos[program] = make(map[ngdp.Region]*ngdp.VersionInfo)
	}

	d.tracking = append(d.tracking, DatastoreTracked{
		Region:  region,
		Program: program,
	})
}

func (d *datastore) Tracking() []DatastoreTracked {
	d.l.RLock()
	defer d.l.RUnlock()

	return d.tracking
}
