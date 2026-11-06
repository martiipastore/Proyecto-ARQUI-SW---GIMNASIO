package domain

type RegistroExterno struct {
	PartnerID, Plataforma, Nombre, Email, ExternalUserID, EventoID, Clave string
	Puntos                                                                int64
	Motivo, Referencia                                                    string
}
type ReciboRegistro struct{ ID, EventoID, ExternalUserID, Estado string }
type CompletarRegistro struct {
	EventoID                                         string
	RegistroID, PartnerID, UsuarioID, ExternalUserID string
	Puntos                                           int64
	Motivo, Referencia                               string
}
