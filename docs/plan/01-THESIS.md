# 01 — Tese, Definição do Produto e Invariantes

## Definição de uma frase

> Um autômato em Go que nunca para, trabalha em três frentes modulares intercaladas com execução cognitiva serial, melhora a si mesmo de forma graduada, reversível e falha-segura versionando no próprio git, calibra o harness à capacidade do modelo disponível (do 70B ao free-tier mais básico), e opera com zero intervenção humana obrigatória, permanecendo 100% observável e interferível em tempo real.

## O que é o EON v2 (estudo de caso)

Um **autômato de software** — não um chatbot, não um framework de agentes genérico. É um sistema vivo de estudo: um processo (mais seu supervisor e workers) que mantém uma missão em progresso contínuo, indefinidamente, sob restrições reais (modelos fracos, cotas gratuitas, falhas, reinícios).

As quatro propriedades que definem o produto:

1. **Continuidade absoluta** — parar é bug ou intervenção autorizada, nunca estado normal. Crash → recupera; espera → persiste a espera; modelo falha → degrada; fila vazia → gera trabalho legítimo; cota esgotada → trabalho determinístico; nada externo (usuário, evento, API) é pré-requisito para existir progresso.
2. **Produz melhorias em três frentes, simultaneamente no tempo lógico, serial na execução**:
   - **F1 PROJETO** — trabalho epistêmico sobre um artefato-alvo definido pela missão (pesquisar, organizar, sintetizar, verificar, evoluir);
   - **F2 PENSAMENTOS** — curadoria da própria memória (comprimir, deduplicar, conectar, reestruturar, revalidar, esquecer supervisionado);
   - **F3 SELF** — melhoria do próprio harness (prompts → políticas → código), graduada, reversível, falha-segura.
3. **Adaptação à capacidade do modelo** — mede o modelo ligado a ele (calibração), atribui nível de capacidade e ajusta o harness: granularidade de decomposição, formato de saída, orçamento de contexto por chamada, rigor de verificação, estratégia de retry. Trocou o modelo? Recalibra e segue.
4. **Autoridade determinística** — o modelo propõe; código determinístico valida, comete e audita. Toda mudança oficial é atômica, reversível e explicável.

## Invariantes fundamentais (a lei do sistema)

1. **Continuidade**: parar só por intervenção autorizada, condição terminal explícita da missão ou falha fatal de storage (nesse caso: incidente registrado + reinício limpo — nunca repouso silencioso). Fila vazia ≠ terminado → geração de trabalho legítimo ou diagnóstico `CONTINUITY_BLOCKED`.
2. **Autoridade determinística**: modelo propõe → parser → validação → commit atômico. O texto do modelo nunca executa nada diretamente. O modelo nunca é o verificador final de si (base: *LLMs Cannot Self-Correct Reasoning Yet*, ICLR 2024).
3. **Falha-segura**: nenhuma melhoria, erro ou injeção pode derrubar o autômato — derruba só a tentativa, que é revertida e analisada. **Falha sem análise registrada é violação de invariante.**
4. **Reversibilidade**: toda mudança da F3 é atômica (1 commit = 1 melhoria), promovida via gates, com janela de observação e rollback automático para o último bom estado.
5. **Serialidade cognitiva**: uma chamada de modelo em voo, sempre. Paralelismo só em capabilities determinísticas, com teto explícito.
6. **Sandbox cego**: exec externa sem rede, sem segredos, filesystem scratch. Conteúdo web é dado, nunca instrução privilegiada.
7. **Orçamento é entrada de scheduling**: esgotou cota → degrada para trabalho determinístico (sleep-time compute), não dorme.
8. **Operador onipotente, nunca obrigatório**: perguntas = demandas pendentes com TTL; nada espera humano para continuar. Intervenção disponível em tempo real, sempre opcional.
9. **Estado fora do modelo**: reiniciar é continuar; contexto é compilado por passo a partir de memória externa (recência × importância × relevância), nunca histórico de chat.
10. **Evidence-or-lesson**: todo outcome vira `Lesson` (positiva ou negativa, com análise) ou `Evidence` verificável — a memória de fracasso é o motor da F3.

## Não-objetivos

- Não é um agente conversacional; não há "chat".
- Não é um framework genérico de agentes; é um organismo específico com uma tese.
- Não depende de modelos fortes, features modernas (tool calling nativo, JSON mode obrigatório) nem de um provedor específico.
- Não busca execução paralela de cognição, multi-agência conversacional, rede P2P, DHT, consenso — lições de deriva de escopo da v1.
- Não tem objetivo econômico próprio; não inventa missão; não amplia autonomia além do teto configurado.
- Não persegue segurança contra adversário com exploits de kernel (fora do modelo de ameaça declarado).

## Métrica central da tese

> O sistema mantém progresso verificável e auditável numa missão real, continuamente, com modelos fracos e orçamento gratuito, degradando com elegância (nunca parando) quando qualquer recurso se esgota — e melhora o próprio harness ao longo do caminho com mais acertos do que regressões (mensurado pela taxa de promoção vs rollback da F3).

A promessa é demonstrada quando: F1 produz conhecimento citado end-to-end com um modelo 7B-class; a F3 promove melhorias reais com janela de observação limpa; e uma semana de operação 24/7 não contém nenhuma parada não-forçada.
