package device

import "time"

// Telemetry holds live runtime sensor and button readings from the controller.
type Telemetry struct {
	BatteryPercent int
	Charging       bool
	FullyCharged   bool

	LeftStickX  float64 // [-1.0, 1.0] raw
	LeftStickY  float64 // [-1.0, 1.0] raw
	RightStickX float64 // [-1.0, 1.0] raw
	RightStickY float64 // [-1.0, 1.0] raw

	L2Pressure float64 // [0.0, 1.0]
	R2Pressure float64 // [0.0, 1.0]

	GyroPitch float64 // degrees [-45..45] normalized for visualization
	GyroYaw   float64
	GyroRoll  float64

	AccelX float64
	AccelY float64
	AccelZ float64

	ButtonsMask    uint32
	MicMutedButton bool
	TouchActive    bool
	TouchX         uint16 // 0..1919
	TouchY         uint16 // 0..1079

	// Diagnostics-grade data (zero values when a backend cannot provide them).
	Buttons        ButtonSet     // buttons held in the most recent report
	ButtonsLatched ButtonSet     // every button seen pressed in any report since the previous poll
	Raw            RawInput      // unprocessed values (no software deadzone or scaling)
	Touch          [2]TouchPoint // multi-touch contacts
	ReportsDropped uint64        // gaps detected in the device's report sequence counter
	ReadErrors     uint64        // failed input reads
	Simulated      bool          // produced by a mock/test device, never real hardware

	LastInputHex  string
	LastOutputHex string
	PacketsRx     uint64
	PacketsTx     uint64
	UpdatedAt     time.Time
}

// RawInput carries unprocessed input values in the device's native units
// (see InputLayout for ranges). Valid is false when the backend has none.
type RawInput struct {
	Valid    bool
	Sticks   [4]int32 // LX, LY, RX, RY
	Triggers [2]int32 // L, R
	Gyro     [3]int16
	Accel    [3]int16
}

// TouchPoint is one touchpad contact.
type TouchPoint struct {
	Active bool
	ID     uint8
	X, Y   uint16
}
