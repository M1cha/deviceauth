// Copyright 2018 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package mongo

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	ctxstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/deviceauth/model"
)

func TestMigration_1_5_0(t *testing.T) {
	pubKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzogVU7RGDilbsoUt/DdH
VJvcepl0A5+xzGQ50cq1VE/Dyyy8Zp0jzRXCnnu9nu395mAFSZGotZVr+sWEpO3c
yC3VmXdBZmXmQdZqbdD/GuixJOYfqta2ytbIUPRXFN7/I7sgzxnXWBYXYmObYvdP
okP0mQanY+WKxp7Q16pt1RoqoAd0kmV39g13rFl35muSHbSBoAW3GBF3gO+mF5Ty
1ddp/XcgLOsmvNNjY+2HOD5F/RX0fs07mWnbD7x+xz7KEKjF+H7ZpkqCwmwCXaf0
iyYyh1852rti3Afw4mDxuVSD7sd9ggvYMc0QHIpQNkD4YWOhNiE1AB0zH57VbUYG
UwIDAQAB
-----END PUBLIC KEY-----`

	ts := time.Now()

	ctx := identity.WithContext(context.Background(), &identity.Identity{
		Tenant: "foo",
	})
	db.Wipe()
	db := NewDataStoreMongoWithSession(db.Session())
	s := db.session

	// prep base version
	mig110 := migration_1_1_0{
		ms:  db,
		ctx: ctx,
	}
	mig120 := migration_1_2_0{
		ms:  db,
		ctx: ctx,
	}
	mig130 := migration_1_3_0{
		ms:  db,
		ctx: ctx,
	}
	mig140 := migration_1_4_0{
		ms:  db,
		ctx: ctx,
	}
	err := mig110.Up(migrate.MakeVersion(1, 1, 0))
	assert.NoError(t, err)
	err = mig120.Up(migrate.MakeVersion(1, 2, 0))
	assert.NoError(t, err)
	err = mig130.Up(migrate.MakeVersion(1, 3, 0))
	assert.NoError(t, err)
	err = mig140.Up(migrate.MakeVersion(1, 4, 0))
	assert.NoError(t, err)

	devs := []model.Device{
		{
			Id:              "1",
			IdData:          "{\"sn\":\"0001\",\"mac\":\"00:00:00:01\"}",
			PubKey:          pubKey,
			Status:          "accepted",
			Decommissioning: false,
			CreatedTs:       ts,
			UpdatedTs:       ts,
		},
		{
			Id:              "2",
			IdData:          "{\"sn\":\"0002\",\"attr\":\"foo1\",\"mac\":\"00:00:00:02\"}",
			PubKey:          pubKey,
			Status:          "pending",
			Decommissioning: false,
			CreatedTs:       ts,
			UpdatedTs:       ts,
		},
		{
			Id:              "3",
			IdData:          "{\"sn\":\"0003\",\"attr\":\"foo3\",\"mac\":\"00:00:00:03\"}",
			PubKey:          pubKey,
			Status:          "rejected",
			Decommissioning: false,
			CreatedTs:       ts,
			UpdatedTs:       ts,
		},
	}

	asets := []model.AuthSet{
		{
			Id:        "1",
			DeviceId:  "1",
			IdData:    "{\"sn\":\"0001\",\"mac\":\"00:00:00:01\"}",
			Status:    "accepted",
			PubKey:    pubKey,
			Timestamp: &ts,
		},
		{
			Id:        "2",
			DeviceId:  "2",
			IdData:    "{\"sn\":\"0002\",\"attr\":\"foo\",\"mac\":\"00:00:00:02\"}",
			Status:    "rejected",
			PubKey:    pubKey,
			Timestamp: &ts,
		},
		{
			Id:        "3",
			DeviceId:  "2",
			IdData:    "{\"sn\":\"0002\",\"attr\":\"foo1\",\"mac\":\"00:00:00:02\"}",
			Status:    "pending",
			PubKey:    pubKey,
			Timestamp: &ts,
		},
		{
			Id:        "4",
			DeviceId:  "3",
			IdData:    "{\"sn\":\"0003\",\"attr\":\"foo3\",\"mac\":\"00:00:00:03\"}",
			Status:    "rejected",
			PubKey:    pubKey,
			Timestamp: &ts,
		},
	}

	for _, d := range devs {
		err = db.AddDevice(ctx, d)
		assert.NoError(t, err)
	}

	for _, a := range asets {
		err = db.AddAuthSet(ctx, a)
		assert.NoError(t, err)
	}

	// test new version
	mig150 := migration_1_5_0{
		ms:  db,
		ctx: ctx,
	}
	err = mig150.Up(migrate.MakeVersion(1, 5, 0))
	assert.NoError(t, err)

	var dev model.Device
	for _, d := range devs {
		err = s.DB(ctxstore.DbFromContext(ctx, DbName)).
			C(DbDevicesColl).FindId(d.Id).One(&dev)

		assert.NoError(t, err)

		status, err := db.GetDeviceStatus(ctx, dev.Id)

		assert.NoError(t, err)
		assert.Equal(t, status, dev.Status)

		d.Status = status
		d.UpdatedTs = dev.UpdatedTs

		decoded, err := decode(dev.IdData)
		assert.NoError(t, err)
		d.IdDataStruct = decoded

		hash := sha256.New()
		hash.Write([]byte(dev.IdData))
		d.IdDataSha256 = hash.Sum(nil)

		compareDevices(&d, &dev, t)
	}

	var set model.AuthSet
	for _, as := range asets {
		err = s.DB(ctxstore.DbFromContext(ctx, DbName)).
			C(DbAuthSetColl).FindId(as.Id).One(&set)
		assert.NoError(t, err)

		as.Timestamp = set.Timestamp

		decoded, err := decode(as.IdData)
		assert.NoError(t, err)
		as.IdDataStruct = decoded

		hash := sha256.New()
		hash.Write([]byte(as.IdData))
		as.IdDataSha256 = hash.Sum(nil)

		compareAuthSet(&as, &set, t)
	}

	db.session.Close()
}
