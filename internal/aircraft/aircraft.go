// Package aircraft adds plane spawn/board/exit with flying-mode emulation.
package aircraft

import (
	"log/slog"
	"sync"

	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/cmd"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
)

const planeID = "aircraft:plane"

var (
	mu     sync.Mutex
	planes = map[uuid.UUID]*world.EntityHandle{}
	pilots = map[uuid.UUID]*flight{}
)

type flight struct {
	plane uuid.UUID
	mode  world.GameMode
	speed float64
}

// Register installs commands and entity types, returning a handler wrapper factory.
func Register(conf *server.Config) func(player.Handler) player.Handler {
	conf.Entities = entity.DefaultRegistry.Config().New(append(entity.DefaultRegistry.Types(), planeType{}))
	cmd.Register(cmd.New("plane", "Spawn, board or leave an aircraft", nil,
		planeSpawn{}, planeBoard{}, planeExit{}))
	slog.Info("aircraft registered", "entity", planeID, "commands", "spawn/board/exit")
	return wrapHandler
}

// planeType is safe as a zero value: every world.EntityType method is
// implemented explicitly and delegates to the text entity backing the plane.
// A nil embedded interface would panic on DecodeNBT while loading saved NBT.
type planeType struct{}

func (planeType) Open(tx *world.Tx, handle *world.EntityHandle, data *world.EntityData) world.Entity {
	return entity.TextType.Open(tx, handle, data)
}
func (planeType) BBox(e world.Entity) cube.BBox { return entity.TextType.BBox(e) }
func (planeType) DecodeNBT(m map[string]any, data *world.EntityData) {
	entity.TextType.DecodeNBT(m, data)
}
func (planeType) EncodeNBT(data *world.EntityData) map[string]any {
	return entity.TextType.EncodeNBT(data)
}

func (planeType) EncodeEntity() string        { return planeID }
func (planeType) NetworkEncodeEntity() string { return "minecraft:minecart" }

type planeSpawn struct {
	Spawn cmd.SubCommand `cmd:"spawn"`
}

func (planeSpawn) Run(src cmd.Source, o *cmd.Output, tx *world.Tx) {
	p := playerFrom(src, o)
	if p == nil || tx == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if h := planes[p.UUID()]; h != nil && !h.Closed() {
		o.Error("Plane already spawned")
		return
	}
	pos := p.Position().Add(mgl64.Vec3{2, -1, 0})
	h := (world.EntitySpawnOpts{Position: pos, NameTag: "AkelCraft Plane"}).New(planeType{}, entity.StationaryBehaviourConfig{})
	tx.AddEntity(h)
	planes[p.UUID()] = h
	slog.Info("aircraft spawned", "player", p.Name(), "plane_uuid", h.UUID(), "pos", pos)
	o.Printf("Plane spawned at %.1f %.1f %.1f", pos[0], pos[1], pos[2])
}

type planeBoard struct {
	Board cmd.SubCommand `cmd:"board"`
}

func (planeBoard) Run(src cmd.Source, o *cmd.Output, tx *world.Tx) {
	p := playerFrom(src, o)
	if p == nil || tx == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if pilots[p.UUID()] != nil {
		o.Error("Already flying a plane")
		return
	}
	h := planes[p.UUID()]
	if h == nil || h.Closed() {
		o.Error("No plane available: use /plane spawn first")
		return
	}
	for _, f := range pilots {
		if f.plane == h.UUID() {
			o.Error("Plane already has a pilot")
			return
		}
	}
	mode := p.GameMode()
	pilots[p.UUID()] = &flight{plane: h.UUID(), mode: mode, speed: p.FlightSpeed()}
	p.SetGameMode(flightMode{mode})
	p.SetFlightSpeed(0.2)
	p.StartFlying()
	slog.Info("aircraft boarded", "player", p.Name(), "plane_uuid", h.UUID(), "flying", p.Flying())
	o.Print("Boarded plane: flying enabled; /plane exit to land")
}

type planeExit struct {
	Exit cmd.SubCommand `cmd:"exit"`
}

func (planeExit) Run(src cmd.Source, o *cmd.Output, _ *world.Tx) {
	p := playerFrom(src, o)
	if p == nil {
		return
	}
	mu.Lock()
	f := pilots[p.UUID()]
	delete(pilots, p.UUID())
	mu.Unlock()
	if f == nil {
		o.Error("Not flying a plane")
		return
	}
	p.StopFlying()
	p.SetGameMode(f.mode)
	p.SetFlightSpeed(f.speed)
	slog.Info("aircraft exited", "player", p.Name(), "plane_uuid", f.plane, "flying", p.Flying())
	o.Print("Left plane; original movement restored")
}

type flightMode struct{ world.GameMode }

func (flightMode) AllowsFlying() bool { return true }
func playerFrom(src cmd.Source, o *cmd.Output) *player.Player {
	p, ok := src.(*player.Player)
	if !ok || p == nil {
		o.Error("only players can use planes")
		return nil
	}
	return p
}

func wrapHandler(next player.Handler) player.Handler {
	if next == nil {
		next = player.NopHandler{}
	}
	return &Handler{Handler: next}
}

type Handler struct{ player.Handler }

func (h *Handler) HandleMove(ctx *player.Context, newPos mgl64.Vec3, newRot cube.Rotation) {
	h.Handler.HandleMove(ctx, newPos, newRot)
	follow(ctx.Player(), ctx.Tx, newPos)
}

func (h *Handler) HandleTeleport(ctx *player.Context, pos mgl64.Vec3) {
	h.Handler.HandleTeleport(ctx, pos)
	follow(ctx.Player(), ctx.Tx, pos)
}

func (h *Handler) HandleQuit(p *player.Player) {
	h.Handler.HandleQuit(p)
	if p == nil {
		return
	}
	mu.Lock()
	f := pilots[p.UUID()]
	delete(pilots, p.UUID())
	delete(planes, p.UUID())
	mu.Unlock()
	if f != nil {
		p.StopFlying()
		p.SetGameMode(f.mode)
		p.SetFlightSpeed(f.speed)
	}
	slog.Info("aircraft quit", "player", p.Name(), "pilot", f != nil)
}

func follow(p *player.Player, tx *world.Tx, pos mgl64.Vec3) {
	if p == nil || tx == nil {
		return
	}
	mu.Lock()
	f, h := pilots[p.UUID()], planes[p.UUID()]
	mu.Unlock()
	if f == nil || h == nil || h.Closed() {
		return
	}
	planePos := pos.Add(mgl64.Vec3{0, -1.2, 0})
	e, ok := h.Entity(tx)
	if ent, ok2 := e.(*entity.Ent); ok && ok2 {
		ent.Teleport(planePos)
		slog.Info("aircraft position changed", "pilot", p.Name(), "player_pos", pos, "plane_pos", planePos)
	}
}
