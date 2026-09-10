package api

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// chatwoot_lid_test.go — o WhatsApp passou a endereçar contatos por LID
// ("196...@lid"), que NÃO é o telefone de ninguém. Estes testes travam o
// comportamento que a loja precisa: o cliente cai no contato/conversa do NÚMERO
// dele, a resposta do painel chega nesse número, e a resposta dada no celular da
// loja entra na conversa DO CLIENTE — nunca numa conversa da loja com ela mesma.

// backendComLID é o backend falso com a tradução LID -> número que a sessão do
// WhatsApp aprende das próprias mensagens.
type backendComLID struct {
	*fakeBackend
	mu      sync.Mutex
	mapa    map[string]string // "196...@lid" -> "5511...@s.whatsapp.net"
	pedidos []string
}

func novoBackendComLID(t *testing.T) *backendComLID {
	t.Helper()
	fb := newFakeBackend()
	_ = fb.Create("bot1")
	return &backendComLID{fakeBackend: fb, mapa: map[string]string{}}
}

func (b *backendComLID) PhoneForLID(name, lid string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pedidos = append(b.pedidos, lid)
	pn, ok := b.mapa[lid]
	return pn, ok
}

func servidorLID(t *testing.T, mock *fullMockChatwoot, be Backend) *Server {
	t.Helper()
	cwsrv := httptest.NewServer(mock.handler())
	t.Cleanup(cwsrv.Close)
	srv := New(Options{APIKey: testKey, Backend: be, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", chatwootConfig{
		Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1",
		URL: cwsrv.URL, MergeBrazilContacts: true, IgnoreJids: []string{"@g.us"},
	})
	return srv
}

// Mensagem endereçada por LID tem de cair no contato do NÚMERO do cliente. Sem
// isso a loja ganha um contato duplicado com um "número" inventado (o painel tem
// 3 contatos assim, ex.: +100000000000005).
func TestEntradaPorLIDUsaONumeroDoCliente(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	be := novoBackendComLID(t)
	be.mapa["100000000000005@lid"] = "56900000001@s.whatsapp.net"
	srv := servidorLID(t, mock, be)

	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "100000000000005@lid", MsgID: "M1", PushName: "Felix", Text: "Hola",
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.createReqs) != 1 {
		t.Fatalf("contatos criados = %d, quer 1", len(mock.createReqs))
	}
	ident, _ := mock.createReqs[0]["identifier"].(string)
	fone, _ := mock.createReqs[0]["phone_number"].(string)
	if strings.Contains(ident, "@lid") {
		t.Errorf("contato nasceu com o LID como identidade: %q", ident)
	}
	if ident != "56900000001@s.whatsapp.net" || fone != "+56900000001" {
		t.Errorf("identidade do contato = %q / %q, quer o número real", ident, fone)
	}
	if len(mock.messages) != 1 {
		t.Fatalf("mensagens gravadas = %d, quer 1", len(mock.messages))
	}
}

// Enquanto a tradução não for conhecida, a mensagem do cliente NÃO pode sumir:
// vai para o painel mesmo com o LID (contato imperfeito é melhor que fala
// perdida).
func TestEntradaPorLIDSemTraducaoNaoPerdeMensagem(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv := servidorLID(t, mock, novoBackendComLID(t))

	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "100000000000005@lid", MsgID: "M2", Text: "Hola",
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.messages) != 1 {
		t.Fatalf("mensagem do cliente se perdeu: %d gravadas", len(mock.messages))
	}
}

// Resposta dada no CELULAR da loja endereçada ao nosso próprio número não pode
// abrir uma conversa da loja com ela mesma — foi o que encheu a conversa 337 do
// painel com respostas dadas a clientes diferentes.
func TestNaoAbreConversaDaLojaComElaMesma(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	be := novoBackendComLID(t)
	be.ownNumber = "56900000000"
	be.mapa["100000000000001@lid"] = "56900000000@s.whatsapp.net"
	srv := servidorLID(t, mock, be)

	srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
		JID: "100000000000001@lid", FromMe: true, MsgID: "M3", Text: "Bom dia",
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.createReqs) != 0 || len(mock.messages) != 0 {
		t.Fatalf("abriu conversa com o próprio número: contatos=%d mensagens=%d", len(mock.createReqs), len(mock.messages))
	}
}

// A mesma mensagem entregue duas vezes ao mesmo tempo (o WhatsApp reentrega
// depois de um pedido de reenvio) só pode virar UMA mensagem no painel.
func TestEntregaSimultaneaNaoDuplicaNoPainel(t *testing.T) {
	mock := newFullMock()
	mock.inboxes = []cwInbox{{ID: 50, Name: "bot1"}}
	srv := servidorLID(t, mock, novoBackendComLID(t))

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.chatwootHandleInbound(context.Background(), "bot1", InboundMessage{
				JID: "5512999998888@s.whatsapp.net", MsgID: "REPETIDA", Text: "Eles não voltam",
			})
		}()
	}
	wg.Wait()

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.messages) != 1 {
		t.Fatalf("mensagem repetida no painel: %d gravadas, quer 1", len(mock.messages))
	}
}

// Resposta do painel para um contato que nasceu de um LID tem de ir para o
// número real do cliente. Antes ela virava "100000000000005@s.whatsapp.net", um
// número que não existe, e o cliente nunca recebia (conversa 557).
func TestRespostaDoPainelParaContatoLIDVaiParaONumero(t *testing.T) {
	chatwootWebhookDelay = 0
	be := novoBackendComLID(t)
	be.mapa["100000000000005@lid"] = "56900000001@s.whatsapp.net"
	srv := New(Options{APIKey: testKey, Backend: be, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", chatwootConfig{Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1"})

	do(t, srv.Handler(), "POST", "/chatwoot/webhook/bot1", "", agentReply("100000000000005@lid", "Buenos dias", ""))

	be.fakeBackend.mu.Lock()
	defer be.fakeBackend.mu.Unlock()
	if len(be.texts) != 1 {
		t.Fatalf("envios = %d, quer 1", len(be.texts))
	}
	if be.texts[0].jid != "56900000001@s.whatsapp.net" {
		t.Fatalf("resposta foi para %q, quer o número real do cliente", be.texts[0].jid)
	}
}

// Contato antigo guardou o LID no lugar do telefone (phone_number
// "+100000000000005", sem identifier): a resposta ainda tem de achar o número.
func TestRespostaParaContatoAntigoComLIDNoCampoTelefone(t *testing.T) {
	chatwootWebhookDelay = 0
	be := novoBackendComLID(t)
	be.mapa["100000000000005@lid"] = "56900000001@s.whatsapp.net"
	srv := New(Options{APIKey: testKey, Backend: be, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", chatwootConfig{Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1"})

	corpo := agentReply("", "Buenos dias", "")
	conv := corpo["conversation"].(map[string]any)
	conv["meta"] = map[string]any{"sender": map[string]any{"phone_number": "+100000000000005"}}
	do(t, srv.Handler(), "POST", "/chatwoot/webhook/bot1", "", corpo)

	be.fakeBackend.mu.Lock()
	defer be.fakeBackend.mu.Unlock()
	if len(be.texts) != 1 || be.texts[0].jid != "56900000001@s.whatsapp.net" {
		t.Fatalf("envios = %+v, quer o número real do cliente", be.texts)
	}
}

// Sem tradução conhecida, a resposta vai para o próprio @lid (que o WhatsApp
// entende) e NUNCA para um "@s.whatsapp.net" inventado.
func TestRespostaSemTraducaoNaoInventaNumero(t *testing.T) {
	chatwootWebhookDelay = 0
	be := novoBackendComLID(t)
	srv := New(Options{APIKey: testKey, Backend: be, Logger: log.New(io.Discard, "", 0)})
	srv.chatwoot.set("bot1", chatwootConfig{Enabled: true, AccountID: "1", Token: "tok", NameInbox: "bot1"})

	do(t, srv.Handler(), "POST", "/chatwoot/webhook/bot1", "", agentReply("100000000000005@lid", "Buenos dias", ""))

	be.fakeBackend.mu.Lock()
	defer be.fakeBackend.mu.Unlock()
	if len(be.texts) != 1 || be.texts[0].jid != "100000000000005@lid" {
		t.Fatalf("envios = %+v, quer o endereço @lid preservado", be.texts)
	}
}

func TestSoDigitosIgnoraAparelhoEDominio(t *testing.T) {
	casos := map[string]string{
		"5512999998888:12@s.whatsapp.net": "5512999998888",
		"100000000000005@lid":              "100000000000005",
		"56900000000":                     "56900000000",
	}
	for entrada, quer := range casos {
		if got := soDigitos(entrada); got != quer {
			t.Errorf("soDigitos(%q) = %q, quer %q", entrada, got, quer)
		}
	}
}
