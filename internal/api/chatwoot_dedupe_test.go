package api

import "testing"

// A mesma mensagem do WhatsApp pode chegar duas vezes quando pedimos reenvio por
// não ter conseguido decifrar a primeira entrega. Ela não pode virar duas
// mensagens no painel — aconteceu em produção, com a equipe vendo a fala do
// cliente repetida na conversa.
func TestMensagemRepetidaNaoEntraDuasVezes(t *testing.T) {
	st := newChatwootMsgStore()
	const instancia = "skichile"
	const waID = "2A445B904DD4131DE2E7"

	if _, existe := st.chatwootIDForWA(instancia, waID); existe {
		t.Fatal("mensagem nova não podia estar registrada")
	}

	st.record(instancia, 4631, waMsgRef{WAID: waID, RemoteJID: "5511@s.whatsapp.net", Text: "Eles não voltam"})

	id, existe := st.chatwootIDForWA(instancia, waID)
	if !existe || id != 4631 {
		t.Fatalf("reentrega devia ser reconhecida: id=%d existe=%v", id, existe)
	}

	// outra instância não pode enxergar o registro da primeira
	if _, existe := st.chatwootIDForWA("outra", waID); existe {
		t.Error("o registro vazou entre instâncias")
	}
}
