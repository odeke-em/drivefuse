// Copyright 2013 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mount

import (
	"os"
	"time"

	"github.com/odeke-em/drivefuse/blob"
	"github.com/odeke-em/drivefuse/metadata"
	"github.com/odeke-em/drivefuse/third_party/code.google.com/p/rsc/fuse"
)

const (
	defaultFileMod = 0774
)

var (
	metaService *metadata.MetaService
	blobManager *blob.Manager
)

type GoogleDriveFS struct{}

func MountAndServe(mountPoint string, meta *metadata.MetaService, blogMngr *blob.Manager) error {
	metaService = meta
	blobManager = blogMngr

	os.MkdirAll(mountPoint, defaultFileMod)
	// try to umount first to cleanup unmounted volumes
	Umount(mountPoint)

	c, err := fuse.Mount(mountPoint)
	if err != nil {
		return err
	}
	c.Serve(GoogleDriveFS{})
	return nil
}

func (GoogleDriveFS) Root() (fuse.Node, fuse.Error) {
	return &GoogleDriveFolder{LocalId: 1}, nil
}

type GoogleDriveFolder struct { // Note: don't change folder terminology
	LocalId       int64
	LocalParentId int64
	Name          string
	Size          int64
	LastMod       time.Time
}

type GoogleDriveFile struct {
	LocalId       int64
	LocalParentId int64
	Name          string
	Md5Checksum   string
	Size          int64
	LastMod       time.Time
}

func (f GoogleDriveFolder) Attr() fuse.Attr {
	return fuse.Attr{
		Mode:  os.ModeDir | defaultFileMod,
		Uid:   uint32(os.Getuid()),
		Gid:   uint32(os.Getgid()),
		Mtime: f.LastMod,
	}
}

func (f GoogleDriveFolder) Lookup(name string, intr fuse.Intr) (fuse.Node, fuse.Error) {
	switch name {
	// ignore some MacOSX lookups
	case "._.", ".hidden", ".DS_Store", "mach_kernel", "Backups.backupdb":
		return nil, fuse.ENOENT
	}

	file, err := metaService.GetChildrenWithName(f.LocalId, name)
	if err != nil || file == nil {
		return nil, fuse.ENOENT
	}
	if file.IsDir {
		return convertToDirNode(file), nil
	}
	return convertToFileNode(file), nil
}

func (f GoogleDriveFolder) Mkdir(req *fuse.MkdirRequest, intr fuse.Intr) (fuse.Node, fuse.Error) {
	file, err := metaService.LocalCreate(f.LocalId, req.Name, 0, true)
	if err != nil {
		return nil, fuse.ENOENT
	}
	return convertToDirNode(file), nil
}

func (f GoogleDriveFolder) Create(req *fuse.CreateRequest, res *fuse.CreateResponse, intr fuse.Intr) (fuse.Node, fuse.Handle, fuse.Error) {
	file, err := metaService.LocalCreate(f.LocalId, req.Name, 0, false)
	if err != nil {
		return nil, nil, fuse.ENOENT
	}
	return convertToFileNode(file), nil, nil
}

func (f GoogleDriveFolder) ReadDir(intr fuse.Intr) ([]fuse.Dirent, fuse.Error) {
	// TODO: handle files with same names under a directory
	ents := []fuse.Dirent{}
	children, _ := metaService.GetChildren(f.LocalId)
	for _, item := range children {
		ents = append(ents, fuse.Dirent{Name: item.Name})
	}
	return ents, nil
}

func (f GoogleDriveFolder) Rename(req *fuse.RenameRequest, newDir fuse.Node, intr fuse.Intr) fuse.Error {
	// TODO: handle files with same names under a directory
	dir := newDir.(*GoogleDriveFolder)
	if err := metaService.LocalMod(f.LocalId, req.OldName, dir.LocalId, req.NewName, -1); err != nil {
		return fuse.EIO
	}
	return nil
}

func (f GoogleDriveFolder) Remove(req *fuse.RemoveRequest, intr fuse.Intr) fuse.Error {
	// TODO: handle files with same names under a directory
	if err := metaService.LocalRm(f.LocalId, req.Name, req.Dir); err != nil {
		return fuse.EIO
	}
	return nil
}

func (f GoogleDriveFile) Attr() fuse.Attr {
	return fuse.Attr{
		Mode:  0400,
		Uid:   uint32(os.Getuid()),
		Gid:   uint32(os.Getgid()),
		Size:  uint64(f.Size),
		Mtime: f.LastMod,
	}
}

func (f GoogleDriveFile) Write(req *fuse.WriteRequest, res *fuse.WriteResponse, intr fuse.Intr) fuse.Error {
	// blob manager and metadata only
	panic("not implemented")
	return nil
}

func (f GoogleDriveFile) Read(req *fuse.ReadRequest, res *fuse.ReadResponse, intr fuse.Intr) fuse.Error {
	var blob []byte
	var err error
	if blob, _, err = blobManager.Read(f.LocalId, f.Md5Checksum, req.Offset, req.Size); err != nil {
		// TODO: add a loading icon and etc
		// TODO: force add the file to the download queue
		return nil
	}
	res.Data = blob
	return nil
}

func convertToDirNode(file *metadata.CachedDriveFile) *GoogleDriveFolder {
	return &GoogleDriveFolder{
		LocalId:       file.LocalId,
		LocalParentId: file.LocalParentId,
		Name:          file.Name,
		LastMod:       file.LastMod}
}

func convertToFileNode(file *metadata.CachedDriveFile) *GoogleDriveFile {
	return &GoogleDriveFile{
		LocalId:       file.LocalId,
		LocalParentId: file.LocalParentId,
		Name:          file.Name,
		Size:          file.FileSize,
		Md5Checksum:   file.Md5Checksum,
		LastMod:       file.LastMod}
}

// TODO(burcud): implement write, release, truncate
