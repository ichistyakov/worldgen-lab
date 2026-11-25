package main

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/customentry"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/category"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

type useModifiersTestItem struct {
	id       string
	duration time.Duration
	modifier float64
}

func (i useModifiersTestItem) EncodeItem() (string, int16)      { return i.id, 0 }
func (i useModifiersTestItem) Name() string                     { return i.id }
func (i useModifiersTestItem) Texture() image.Image             { return image.NewNRGBA(image.Rect(0, 0, 1, 1)) }
func (i useModifiersTestItem) Category() category.Category      { return category.Items() }
func (i useModifiersTestItem) AlwaysConsumable() bool           { return true }
func (i useModifiersTestItem) ConsumeDuration() time.Duration   { return i.duration }
func (i useModifiersTestItem) Consume(*world.Tx, item.Consumer) item.Stack { return item.Stack{} }
func (i useModifiersTestItem) MovementModifier() float64        { return i.modifier }

func testUseModifierNumber(t *testing.T, v any) float64 {
	t.Helper()
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int32:
		return float64(x)
	case int:
		return float64(x)
	default:
		t.Fatalf("value %#v is not a number (%T)", v, v)
		return 0
	}
}

// TestGenericUseModifiersComponent proves the public customentry.Item entry for
// a custom item exposes minecraft:use_modifiers to clients in the item registry.
func TestGenericUseModifiersComponent(t *testing.T) {
	it := useModifiersTestItem{"akelcraft:generic-use-modifiers", 1500 * time.Millisecond, 0.25}
	entry := customentry.Item(it, 42)
	if entry.Name != "akelcraft:generic-use-modifiers" || entry.RuntimeID != 42 {
		t.Fatalf("unexpected entry identity: %#v", entry)
	}
	if !entry.ComponentBased || entry.Version != protocol.ItemEntryVersionDataDriven {
		t.Fatalf("entry is not component-based data-driven: %#v", entry)
	}
	components, ok := entry.Data["components"].(map[string]any)
	if !ok {
		t.Fatalf("entry Data has no components map: %#v", entry.Data)
	}
	modifiers, ok := components["minecraft:use_modifiers"].(map[string]any)
	if !ok {
		t.Fatalf("minecraft:use_modifiers missing from registry entry: %#v", components)
	}
	if got := testUseModifierNumber(t, modifiers["movement_modifier"]); math.Abs(got-0.25) > 1e-9 {
		t.Errorf("movement_modifier = %v, want 0.25", got)
	}
	if got := testUseModifierNumber(t, modifiers["use_duration"]); math.Abs(got-1.5) > 1e-9 {
		t.Errorf("use_duration = %v, want 1.5 seconds", got)
	}
}
