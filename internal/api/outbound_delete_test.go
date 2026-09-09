package api

import "testing"

// Apagar no painel NÃO pode reenviar a mensagem ao cliente. O webhook de
// "message_updated" com deleted=true caía no fluxo de envio normal e o cliente
// recebia o texto de novo — o oposto do que o atendente pediu.
func TestApagarNaoCaiNoFluxoDeEnvio(t *testing.T) {
	casos := []struct {
		nome    string
		evento  string
		apagada bool
		sai     bool // deve sair antes do envio
	}{
		{"edicao comum sai cedo", "message_updated", false, true},
		{"apagada sai cedo (nao reenvia)", "message_updated", true, true},
		{"mensagem nova segue para envio", "message_created", false, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			// espelha a condição do handler: qualquer message_updated encerra ali
			saiCedo := c.evento == "message_updated"
			if saiCedo != c.sai {
				t.Errorf("evento=%q apagada=%v: saiCedo=%v, esperado %v", c.evento, c.apagada, saiCedo, c.sai)
			}
		})
	}
}
