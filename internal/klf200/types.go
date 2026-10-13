package klf200

import (
	"fmt"
	"time"
)

// Command IDs for KLF-200 protocol
type CommandID uint16

// Command IDs as defined in the Velux KLF-200 Technical Specification
// (and cross-checked against the pyvlx reference implementation).
const (
	// Error notification
	GW_ERROR_NTF CommandID = 0x0000

	// Reboot
	GW_REBOOT_REQ CommandID = 0x0001
	GW_REBOOT_CFM CommandID = 0x0002

	// Gateway state (used as keep-alive)
	GW_GET_STATE_REQ CommandID = 0x000C
	GW_GET_STATE_CFM CommandID = 0x000D

	// Authentication
	GW_PASSWORD_ENTER_REQ CommandID = 0x3000
	GW_PASSWORD_ENTER_CFM CommandID = 0x3001

	// Single node information
	GW_GET_NODE_INFORMATION_REQ CommandID = 0x0200
	GW_GET_NODE_INFORMATION_CFM CommandID = 0x0201
	GW_GET_NODE_INFORMATION_NTF CommandID = 0x0210

	// All nodes discovery
	GW_GET_ALL_NODES_INFORMATION_REQ          CommandID = 0x0202
	GW_GET_ALL_NODES_INFORMATION_CFM          CommandID = 0x0203
	GW_GET_ALL_NODES_INFORMATION_NTF          CommandID = 0x0204
	GW_GET_ALL_NODES_INFORMATION_FINISHED_NTF CommandID = 0x0205

	// State changes
	GW_NODE_STATE_POSITION_CHANGED_NTF CommandID = 0x0211

	// House status monitor
	GW_HOUSE_STATUS_MONITOR_ENABLE_REQ  CommandID = 0x0240
	GW_HOUSE_STATUS_MONITOR_ENABLE_CFM  CommandID = 0x0241
	GW_HOUSE_STATUS_MONITOR_DISABLE_REQ CommandID = 0x0242
	GW_HOUSE_STATUS_MONITOR_DISABLE_CFM CommandID = 0x0243

	// Commands
	GW_COMMAND_SEND_REQ           CommandID = 0x0300
	GW_COMMAND_SEND_CFM           CommandID = 0x0301
	GW_COMMAND_RUN_STATUS_NTF     CommandID = 0x0302
	GW_COMMAND_REMAINING_TIME_NTF CommandID = 0x0303
	GW_SESSION_FINISHED_NTF       CommandID = 0x0304

	// Status
	GW_STATUS_REQUEST_REQ CommandID = 0x0305
	GW_STATUS_REQUEST_CFM CommandID = 0x0306
	GW_STATUS_REQUEST_NTF CommandID = 0x0307

	// Limitation status (sensors: rain, wind, etc.)
	GW_SET_LIMITATION_REQ        CommandID = 0x0310
	GW_SET_LIMITATION_CFM        CommandID = 0x0311
	GW_GET_LIMITATION_STATUS_REQ CommandID = 0x0312
	GW_GET_LIMITATION_STATUS_CFM CommandID = 0x0313
	GW_LIMITATION_STATUS_NTF     CommandID = 0x0314
)

// NodeType represents the combined node type and sub type of a Velux device.
//
// The value is laid out as (type << 6) | subtype. The constants below list the
// documented combinations; String() falls back to the base type for unknown
// sub types.
type NodeType uint16

const (
	NodeTypeInteriorVenetianBlind            NodeType = 0x0040
	NodeTypeRollerShutter                    NodeType = 0x0080
	NodeTypeAdjustableSlatsRollerShutter     NodeType = 0x0081
	NodeTypeAdjustableSlatsRollerShutterProj NodeType = 0x0082
	NodeTypeVerticalExteriorAwning           NodeType = 0x00C0
	NodeTypeWindowOpener                     NodeType = 0x0100
	NodeTypeWindowOpenerWithRainSensor       NodeType = 0x0101
	NodeTypeGarageDoorOpener                 NodeType = 0x0140
	NodeTypeGarageDoorOpenerLinearAngular    NodeType = 0x017A
	NodeTypeLight                            NodeType = 0x0180
	NodeTypeLightOnOff                       NodeType = 0x01BA
	NodeTypeGateOpener                       NodeType = 0x01C0
	NodeTypeGateOpenerAngularPosition        NodeType = 0x01FA
	NodeTypeDoorLock                         NodeType = 0x0240
	NodeTypeWindowLock                       NodeType = 0x0241
	NodeTypeVerticalInteriorBlinds           NodeType = 0x0280
	NodeTypeDualRollerShutter                NodeType = 0x0340
	NodeTypeOnOffSwitch                      NodeType = 0x03C0
	NodeTypeHorizontalAwning                 NodeType = 0x0400
	NodeTypeExteriorVenetianBlind            NodeType = 0x0440
	NodeTypeLouverBlind                      NodeType = 0x0480
	NodeTypeCurtainTrack                     NodeType = 0x04C0
	NodeTypeVentilationPoint                 NodeType = 0x0500
	NodeTypeVentilationPointAirInlet         NodeType = 0x0501
	NodeTypeVentilationPointAirTransfer      NodeType = 0x0502
	NodeTypeVentilationPointAirOutlet        NodeType = 0x0503
	NodeTypeExteriorHeating                  NodeType = 0x0540
	NodeTypeSwingingShutters                 NodeType = 0x0600
	NodeTypeSwingingShutterIndependentLeaves NodeType = 0x0601
	NodeTypeBladeOpener                      NodeType = 0x0740
)

// BaseType returns the base node type (upper 10 bits) without the sub type.
func (t NodeType) BaseType() uint16 {
	return uint16(t) >> 6
}

// baseTypeNames maps the base node type (value >> 6) to a display name.
var baseTypeNames = map[uint16]string{
	1:  "Interior Venetian Blind",
	2:  "Roller Shutter",
	3:  "Vertical Exterior Awning",
	4:  "Window Opener",
	5:  "Garage Door Opener",
	6:  "Light",
	7:  "Gate Opener",
	8:  "Rolling Door Opener",
	9:  "Door Lock",
	10: "Vertical Interior Blinds",
	11: "Secure Configuration Device",
	12: "Beacon",
	13: "Dual Roller Shutter",
	14: "Heating Temperature Interface",
	15: "On/Off Switch",
	16: "Horizontal Awning",
	17: "Exterior Venetian Blind",
	18: "Louver Blind",
	19: "Curtain Track",
	20: "Ventilation Point",
	21: "Exterior Heating",
	22: "Heat Pump",
	23: "Intrusion Alarm",
	24: "Swinging Shutter",
	29: "Blade Opener",
}

// String returns a human-readable name for the node type. Known sub types get
// a specific name, all other values fall back to the base type name.
func (t NodeType) String() string {
	switch t {
	case NodeTypeAdjustableSlatsRollerShutter:
		return "Roller Shutter (adjustable slats)"
	case NodeTypeAdjustableSlatsRollerShutterProj:
		return "Roller Shutter (adjustable slats, projection)"
	case NodeTypeWindowOpenerWithRainSensor:
		return "Window Opener (rain sensor)"
	case NodeTypeWindowLock:
		return "Window Lock"
	case NodeTypeLightOnOff:
		return "Light (on/off)"
	case NodeTypeVentilationPointAirInlet:
		return "Ventilation Point (air inlet)"
	case NodeTypeVentilationPointAirTransfer:
		return "Ventilation Point (air transfer)"
	case NodeTypeVentilationPointAirOutlet:
		return "Ventilation Point (air outlet)"
	case NodeTypeSwingingShutterIndependentLeaves:
		return "Swinging Shutter (independent leaves)"
	}
	if name, ok := baseTypeNames[t.BaseType()]; ok {
		return name
	}
	return fmt.Sprintf("Unknown (0x%04X)", uint16(t))
}

// NodeState represents the operational state of a node
type NodeState uint8

const (
	NodeStateNonExecuting        NodeState = 0
	NodeStateErrorWhileExecution NodeState = 1
	NodeStateNotUsed             NodeState = 2
	NodeStateWaitingForPower     NodeState = 3
	NodeStateExecuting           NodeState = 4
	NodeStateDone                NodeState = 5
	NodeStateUnknown             NodeState = 255
)

func (s NodeState) String() string {
	switch s {
	case NodeStateNonExecuting:
		return "Non-Executing"
	case NodeStateErrorWhileExecution:
		return "Error"
	case NodeStateNotUsed:
		return "Not Used"
	case NodeStateWaitingForPower:
		return "Waiting for Power"
	case NodeStateExecuting:
		return "Executing"
	case NodeStateDone:
		return "Done"
	default:
		return "Unknown"
	}
}

// RunStatus represents the run status of a command
type RunStatus uint8

const (
	RunStatusExecutionCompleted RunStatus = 0
	RunStatusExecutionFailed    RunStatus = 1
	RunStatusExecutionActive    RunStatus = 2
)

// StatusReply represents the status reply type
type StatusReply uint8

const (
	StatusReplyUnknownStatusReply            StatusReply = 0x00
	StatusReplyCommandCompletedOk            StatusReply = 0x01
	StatusReplyNoContact                     StatusReply = 0x02
	StatusReplyManuallyOperated              StatusReply = 0x03
	StatusReplyBlocked                       StatusReply = 0x04
	StatusReplyWrongSystemKey                StatusReply = 0x05
	StatusReplyPriorityLevelLocked           StatusReply = 0x06
	StatusReplyReachedWrongPosition          StatusReply = 0x07
	StatusReplyErrorDuringExecution          StatusReply = 0x08
	StatusReplyNoExecution                   StatusReply = 0x09
	StatusReplyCalibrating                   StatusReply = 0x0A
	StatusReplyPowerConsumptionTooHigh       StatusReply = 0x0B
	StatusReplyPowerConsumptionTooLow        StatusReply = 0x0C
	StatusReplyLockPositionOpen              StatusReply = 0x0D
	StatusReplyMotionTimeTooLongCommunError  StatusReply = 0x0E
	StatusReplyThermalProtection             StatusReply = 0x0F
	StatusReplyProductNotOperational         StatusReply = 0x10
	StatusReplyFilterMaintenanceNeeded       StatusReply = 0x11
	StatusReplyBatteryLevel                  StatusReply = 0x12
	StatusReplyTargetModified                StatusReply = 0x13
	StatusReplyModeNotImplemented            StatusReply = 0x14
	StatusReplyCommandIncompatibleToMovement StatusReply = 0x15
	StatusReplyUserAction                    StatusReply = 0x16
	StatusReplyDeadBoltError                 StatusReply = 0x17
	StatusReplyAutomaticCycleEngaged         StatusReply = 0x18
	StatusReplyWrongLoadConnected            StatusReply = 0x19
	StatusReplyColourNotReachable            StatusReply = 0x1A
	StatusReplyTargetNotReachable            StatusReply = 0x1B
	StatusReplyBadIndexReceived              StatusReply = 0x1C
	StatusReplyCommandOverruled              StatusReply = 0x1D
	StatusReplyNodeWaitingForPower           StatusReply = 0x1E
	StatusReplyInformationCode               StatusReply = 0xDF
	StatusReplyParameterLimited              StatusReply = 0xE0
	StatusReplyLimitationByLocalUser         StatusReply = 0xE1
	StatusReplyLimitationByUser              StatusReply = 0xE2
	StatusReplyLimitationByRain              StatusReply = 0xE3
	StatusReplyLimitationByTimer             StatusReply = 0xE4
	StatusReplyLimitationByUPS               StatusReply = 0xE6
	StatusReplyLimitationByUnknown           StatusReply = 0xE7
	StatusReplyLimitationBySAAC              StatusReply = 0xEA
	StatusReplyLimitationByWind              StatusReply = 0xEB
	StatusReplyLimitationByMyself            StatusReply = 0xEC
	StatusReplyLimitationByAutomaticCycle    StatusReply = 0xED
	StatusReplyLimitationByEmergency         StatusReply = 0xEE
)

// Velocity represents movement speed
type Velocity uint8

const (
	VelocityDefault Velocity = 0
	VelocitySilent  Velocity = 1
	VelocityFast    Velocity = 2
	VelocityNotUsed Velocity = 255
)

// Priority level for commands
type Priority uint8

const (
	PriorityHumanProtection       Priority = 0
	PriorityEnvironmentProtection Priority = 1
	PriorityUserLevel1            Priority = 2
	PriorityUserLevel2            Priority = 3
	PriorityComfortLevel1         Priority = 4
	PriorityComfortLevel2         Priority = 5
	PriorityComfortLevel3         Priority = 6
	PriorityComfortLevel4         Priority = 7
)

// Special main-parameter values (KLF-200 Technical Specification, "Parameter value").
const (
	PositionMin     uint16 = 0x0000 // Fully open
	PositionMax     uint16 = 0xC800 // Fully closed (51200)
	PositionTarget  uint16 = 0xD100 // Use the stored target value
	PositionCurrent uint16 = 0xD200 // Keep current position (= stop)
	PositionDefault uint16 = 0xD300 // Use default position
	PositionIgnore  uint16 = 0xD400 // Ignore this parameter
	PositionUnknown uint16 = 0xF7FF // Value unknown / not set
)

// Node represents a Velux device
type Node struct {
	ID              uint8     `json:"id"`
	Name            string    `json:"name"`
	NodeType        NodeType  `json:"node_type"`
	NodeTypeStr     string    `json:"node_type_str"`
	State           NodeState `json:"state"`
	StateStr        string    `json:"state_str"`
	CurrentPosition uint16    `json:"current_position_raw"`
	PositionPercent float64   `json:"position_percent"`
	TargetPosition  uint16    `json:"target_position_raw"`
	TargetPercent   float64   `json:"target_percent"`
	Velocity        Velocity  `json:"velocity"`
	LastUpdate      time.Time `json:"last_update"`
	Inverted        bool      `json:"inverted"` // true for window openers (0%=closed, 100%=open)

	// PositionValid indicates that CurrentPosition/TargetPosition carry real
	// data. State-only updates (e.g. from GW_COMMAND_RUN_STATUS_NTF) leave it
	// false so the cached position is not overwritten with zeros.
	PositionValid bool `json:"-"`
}

// IsInvertedType returns true for node types where position semantics are inverted
// For Velux/io-homecontrol: 0% = open, 100% = closed for all device types
// (window openers, shutters, blinds all use the same semantics)
func (t NodeType) IsInvertedType() bool {
	// All Velux devices use: 0% = open, 100% = closed
	return false
}

// PositionToPercent converts raw position (0-51200) to percentage (0-100)
// 0 = fully open, 100 = fully closed
func PositionToPercent(raw uint16) float64 {
	if raw > PositionMax {
		return 100.0
	}
	return float64(raw) / float64(PositionMax) * 100.0
}

// PercentToPosition converts percentage (0-100) to raw position (0-51200)
func PercentToPosition(percent float64) uint16 {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return uint16(percent / 100.0 * float64(PositionMax))
}

// Frame represents a KLF-200 protocol frame
type Frame struct {
	Command CommandID
	Data    []byte
}

// Response status codes
type ResponseStatus uint8

const (
	StatusOK                ResponseStatus = 0
	StatusErrorSystem       ResponseStatus = 1
	StatusErrorInvalidIndex ResponseStatus = 2
	StatusErrorOutOfRange   ResponseStatus = 3
)

// LimitationOriginator identifies who set a limitation on a node. The values
// are the "Command Originator" enumeration of the KLF-200 specification.
type LimitationOriginator uint8

const (
	LimitationOriginatorUser         LimitationOriginator = 1   // User remote control
	LimitationOriginatorRain         LimitationOriginator = 2   // Rain sensor
	LimitationOriginatorTimer        LimitationOriginator = 3   // Timer controlled
	LimitationOriginatorUPS          LimitationOriginator = 5   // UPS unit
	LimitationOriginatorSAAC         LimitationOriginator = 8   // Stand Alone Automatic Controls
	LimitationOriginatorWind         LimitationOriginator = 9   // Wind sensor
	LimitationOriginatorLoadShedding LimitationOriginator = 11  // Load shedding
	LimitationOriginatorLocalLight   LimitationOriginator = 12  // Local light sensor
	LimitationOriginatorEnvSensor    LimitationOriginator = 13  // Unspecific environment sensor
	LimitationOriginatorEmergency    LimitationOriginator = 255 // Emergency / security
)

func (l LimitationOriginator) String() string {
	switch l {
	case LimitationOriginatorUser:
		return "User"
	case LimitationOriginatorRain:
		return "Rain"
	case LimitationOriginatorTimer:
		return "Timer"
	case LimitationOriginatorUPS:
		return "UPS"
	case LimitationOriginatorSAAC:
		return "SAAC"
	case LimitationOriginatorWind:
		return "Wind"
	case LimitationOriginatorLoadShedding:
		return "Load Shedding"
	case LimitationOriginatorLocalLight:
		return "Local Light"
	case LimitationOriginatorEnvSensor:
		return "Environment Sensor"
	case LimitationOriginatorEmergency:
		return "Emergency"
	default:
		return fmt.Sprintf("Unknown (%d)", uint8(l))
	}
}

// LimitationStatus represents the limitation status for a node
type LimitationStatus struct {
	SessionID        uint16               `json:"session_id"`
	NodeID           uint8                `json:"node_id"`
	MinValue         uint16               `json:"min_value"`
	MaxValue         uint16               `json:"max_value"`
	LimitationOrigin LimitationOriginator `json:"limitation_origin"`
	LimitationTime   uint8                `json:"limitation_time"` // 0-252: n*30s, 253: unlimited
}

// Active reports whether this notification describes an active limitation.
// A node without limitation reports "unknown" (0xF7FF) for both bounds.
func (l *LimitationStatus) Active() bool {
	return l.MinValue != PositionUnknown || l.MaxValue != PositionUnknown
}

// SensorStatus represents the current sensor readings
type SensorStatus struct {
	RainDetected bool      `json:"rain_detected"`
	WindDetected bool      `json:"wind_detected"`
	LastUpdate   time.Time `json:"last_update"`
}
