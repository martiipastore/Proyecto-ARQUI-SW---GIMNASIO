package model

import "errors"

// ErrDuplicate indica DNI o email ya registrados (RN-33, CA-01.2).
var ErrDuplicate = errors.New("persona duplicada")

// ErrAssignmentDuplicate indica una asignación vigente igual (SPEC §9).
var ErrAssignmentDuplicate = errors.New("asignación duplicada")

// Person es una persona del padrón: alumno, profesional o administrador. Las
// personas externas del Club (D-63) no tienen DNI hasta que el administrador
// las da de alta como alumnos (D-78).
type Person struct {
	ID              string  `json:"id"`
	Role            string  `json:"rol"`
	Name            string  `json:"nombre"`
	LastName        *string `json:"apellido"`
	DNI             *string `json:"dni"`
	Email           string  `json:"email"`
	Phone           *string `json:"telefono"`
	BirthDate       *string `json:"fecha_nacimiento"`
	License         *string `json:"matricula"`
	State           string  `json:"estado"`
	External        bool    `json:"registro_externo"`
	CreatedAt       string  `json:"fecha_alta"`
	DeactivatedAt   *string `json:"dado_de_baja_en"`
	PendingPassword bool    `json:"activacion_pendiente"`
}

// PersonInput son los datos de alta (HU-01, HU-02 y SPEC §9).
type PersonInput struct {
	Role      string `json:"rol"`
	Name      string `json:"nombre"`
	LastName  string `json:"apellido"`
	DNI       string `json:"dni"`
	Email     string `json:"email"`
	Phone     string `json:"telefono"`
	BirthDate string `json:"fecha_nacimiento"`
	License   string `json:"matricula"`
}

// PersonUpdate son los datos modificables; DNI, email y rol no cambian.
type PersonUpdate struct {
	Name      string `json:"nombre"`
	LastName  string `json:"apellido"`
	Phone     string `json:"telefono"`
	BirthDate string `json:"fecha_nacimiento"`
	License   string `json:"matricula"`
}

type PersonFilter struct {
	Role, State, Query string
	Page, Size         int
}

type PersonPage struct {
	Items []Person `json:"items"`
	Total int64    `json:"total"`
	Page  int      `json:"pagina"`
	Size  int      `json:"tamano"`
}

// StudentSummary son los datos mínimos que comparten otros servicios.
type StudentSummary struct {
	ID       string  `json:"id"`
	Name     string  `json:"nombre"`
	LastName *string `json:"apellido"`
	State    string  `json:"estado"`
}

// Assignment vincula un profesional con un alumno (RN-30, HU-02).
type Assignment struct {
	ID             string        `json:"id"`
	ProfessionalID string        `json:"profesional_id"`
	StudentID      string        `json:"alumno_id"`
	Since          string        `json:"desde"`
	Until          *string       `json:"hasta"`
	Student        *PersonDetail `json:"alumno,omitempty"`
	Professional   *PersonDetail `json:"profesional,omitempty"`
}

// PersonDetail son los datos de contacto visibles para administración y para el
// profesional asignado (RN-30).
type PersonDetail struct {
	ID       string  `json:"id"`
	Name     string  `json:"nombre"`
	LastName *string `json:"apellido"`
	Email    string  `json:"email"`
	Phone    *string `json:"telefono"`
	Role     string  `json:"rol"`
	State    string  `json:"estado"`
}
