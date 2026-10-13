// Package port define as interfaces estreitas por repositório/capability
// (docs/plan/02-ARCHITECTURE.md, regra 2; ADR-0002).
//
// Lei: port importa só stdlib + domain. Cada contrato é ESTREITO — uma
// interface por repositório, uma por capability. A anti-god-interface da v1
// (internal/port/storage.go com a união de tudo) é a vergonha canônica que
// este pacote existe para nunca repetir.
//
// M0: nada precisa de interface ainda — os primeiros contratos (storage,
// model, WorkFront, sandbox probe) nascem no M1/M2 QUANDO existirem dois
// lados que precisem deles. Interface sem segundo lado é invenção.
package port
