package aircraft

import (
	"testing"

	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/mcdb"
	"github.com/go-gl/mathgl/mgl64"
)

// TestPlaneTypeSurvivesMCDBSaveAndReload covers the exact crash path: a plane
// spawned with the zero-value planeType is saved by mcdb, then the chunk is
// loaded in a new world whose entity registry is the one built by Register. The registered
// type must be able to decode its own NBT.
func TestPlaneTypeSurvivesMCDBSaveAndReload(t *testing.T) {
	var conf server.Config
	Register(&conf)

	dir := t.TempDir()
	db, err := mcdb.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := (world.Config{
		Provider:     db,
		Entities:     conf.Entities,
		Synchronous:  true,
		SaveInterval: -1,
	}).New()
	pos := mgl64.Vec3{8, 64, 8}
	task := w.Do(func(tx *world.Tx) {
		tx.AddEntity((world.EntitySpawnOpts{Position: pos, NameTag: "saved plane"}).
			New(planeType{}, entity.StationaryBehaviourConfig{}))
	})
	if err := task.Err(); err != nil {
		t.Fatal(err)
	}
	// World.Close saves all loaded chunks and closes the provider itself.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Confirm the plane really reached LevelDB as aircraft:plane NBT.
	checkDB, err := mcdb.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	col, err := checkDB.LoadColumn(world.ChunkPos{}, world.Overworld)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range col.Entities {
		if e.Data["identifier"] == planeID {
			found = true
		}
	}
	if !found {
		t.Fatalf("saved column has no %s actor: %+v", planeID, col.Entities)
	}
	if err := checkDB.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen with the same registry Register installs on a real server. This
	// load previously panicked in world.entityFromData -> planeType.DecodeNBT.
	db, err = mcdb.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w = (world.Config{
		Provider:     db,
		Entities:     conf.Entities,
		Synchronous:  true,
		SaveInterval: -1,
	}).New()
	defer func() { _ = w.Close() }()
	task = w.Do(func(tx *world.Tx) {
		// Reading a block forces the saved column (and its actors) to load.
		_ = tx.Block(cube.Pos{8, 64, 8})
		var planes int
		for e := range tx.Entities() {
			if e.H().Type().EncodeEntity() == planeID {
				planes++
				netType, ok := e.H().Type().(interface{ NetworkEncodeEntity() string })
				if !ok || netType.NetworkEncodeEntity() != "minecraft:minecart" {
					t.Errorf("reloaded plane network id = %v", e.H().Type())
				}
			}
		}
		if planes != 1 {
			t.Errorf("reloaded planes = %d, want 1", planes)
		}
	})
	if err := task.Err(); err != nil {
		t.Fatal(err)
	}
}
