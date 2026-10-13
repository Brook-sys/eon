# 04 — Fundamentação Científica e Referências

> Este documento é a base empírica do desenho. Também é a **lista de leitura semente da primeira missão da F1** (D21): o autômato nasce estudando a literatura que o fundamenta.

## Q1 — "Um sistema pode aprender continuamente, 24/7, por anos?"

| Evidência | Achado | Implicação no EON |
|---|---|---|
| **NELL** — Never-Ending Language Learner (Mitchell et al., CACM 2018; paradigma em AAAI 2018) | Rodando 24h/dia desde jan/2010; 120M crenças ponderadas por confiança; milhares de tarefas de aprendizado acopladas | O precedente direto. Lições: (a) crenças falsas **se acumulam** → confiança ponderada + revalidação periódica (Ciclo 2/F2); (b) **acoplamento entre tarefas** impede erro composto → frontier e Lessons compartilhadas entre frentes; (c) paradigma formal ⟨L=tarefas crescentes, C=restrições de acoplamento⟩ → a definição teórica da nossa agenda renovável |

## Q2 — "Modelos fracos + harness forte realmente funcionam?"

| Evidência | Achado | Implicação |
|---|---|---|
| **TinyGSM** (Liu et al., 2023, arXiv:2312.09241) | 1.3B gerador + 1.3B verificador = **81,5%** no GSM8K, superando modelos 10× maiores sem verificador | O resultado-assinatura da tese: competência vem do **par gerador+verificador**, não do tamanho. No EON: gerador = modelo fraco; verificador = código determinístico/executor |
| **Large Language Monkeys** (Brown et al., 2024, arXiv:2407.21787) | Cobertura escala **log-linearmente** com nº de amostras (4 ordens de magnitude); DeepSeek-Coder-V2: 15,9%→56% no SWE-bench Lite com 250 amostras; 5 amostras de modelo fraco > 1 amostra de GPT-4o a 1/3 do custo | `n-sampling + verificação` como estratégia do cognition module quando o orçamento permite |
| **Let's Verify Step by Step** (Lightman et al., OpenAI, 2023, arXiv:2305.20050) | Supervisão de **processo** (por passo) > supervisão de resultado (78,2% vs 72,4% no MATH); active learning = 2,6× eficiência | Microturnos com **verificação por passo** (Ciclo 0) e seleção de candidatos por ganho de informação |
| **τ-bench** (2024) + estudos de longa-horizonte | GPT-4o: <50% pass@1, <25% pass^8 em tarefas longas com ferramentas; pass@1 cai 76,3%→52,1% em tarefas longuíssimas; 95% por passo × 20 passos = 36% total | Mesmo modelos fortes falham em cadeias longas → estado fora do modelo, passos curtos, verificação por passo, **consistência (pass^k) > sorte (pass@1)** |
| **LLM-Modulo** (Kambhampati et al., ICML 2024, arXiv:2402.01817) | LLMs não planejam sozinhos; são fontes de conhecimento aproximadas acopladas a **verificadores externos** em interação bidirecional | O EON é uma instância de LLM-Modulo aplicada a um runtime contínuo: proposta→validação→commit |
| **AI Parenting: Grounding LLM Agents via Self-Inquiry** (Google Cloud AI Research, 2025; ID arXiv a confirmar — A09) | Variante Socrática: gerador fraco + verificação rara-mas-forte; Llama-2 70B ≈ GPT-4 no MMLU dentro do framework | Evidência de que o padrão funciona com modelos de geração anterior — exatamente o alvo do EON |

## Q3 — "Como aprender sem fine-tuning?"

| Evidência | Achado | Implicação |
|---|---|---|
| **Reflexion** (Shinn et al., NeurIPS 2023, arXiv:2303.11366) | "Reforço verbal": feedback → reflexão linguística em memória episódica → próxima tentativa; pesos não mudam | Base do `Lesson` (Ciclo 1): falha → diagnóstico citado → memória consultável |
| **ExpeL** (Zhao et al., AAAI 2024, arXiv:2308.10144) | Pool de experiências (sucessos E fracassos) → extração de insights → recall; transferência entre domínios | Lessons sintetizadas em **regras gerais** consultáveis; fracasso é matéria-prima |
| **Generative Agents** (Park et al., UIST 2023, arXiv:2304.03442) | Memory stream + recuperação **recência × importância × relevância** + reflexões periódicas em árvore; ablações provam cada peça crítica | O blueprint da frente F2 e do ContextCompiler (Ciclo 0). Relevância por FTS5/BM25 primeiro; embeddings depois |
| **Voyager** (Wang et al., 2023, arXiv:2305.16291) | Currículo automático + **biblioteca de habilidades** (código testado, composicional) + iteração com feedback do ambiente | F3 nível L1: capacidades que passam por teste viram skills versionadas, compostas, reutilizáveis |
| **Sleep-time Compute** (Lin et al., Letta/UC Berkeley, 2025, arXiv:2504.13171) | Raciocínio em tempo ocioso **amortiza custo** e melhora a resposta por consulta; reescrita de memória no idle | Cota esgotada ≠ pausa: o Ciclo 2 **acelera** (D12, D22) |

## Q4 — "Auto-melhoria segura é possível?"

| Evidência | Achado | Implicação |
|---|---|---|
| **Gödel Machine** (Schmidhuber, 2003) | Self-modificação com **prova** de benefício líquido — teoricamente lindo, **impraticável** | Explica por que precisamos do approach empírico |
| **Darwin Gödel Machine** (Zhang, Hu, Lu, Lange, Clune — Sakana/UBC, 2025, arXiv:2505.22954) | Substitui prova por **validação empírica + arquivo de variantes** (open-endedness); dobrou SWE-bench; melhorias transferem | O protocolo da F3: sandbox (validação) + `auto/*` (linhagem/arquivo, inclusive rejeitadas) + janela de observação + rollback. Com modelos fracos: propostas **pequenas**, gates determinísticos, nunca autoavaliação do modelo como juiz |

## Q5 — "Como gerar trabalho legítimo sem busywork?"

| Evidência | Achado | Implicação |
|---|---|---|
| **Schmidhuber** — teoria formal de curiosidade/interestingness | Recompensa intrínseca = **progresso de aprendizagem/compressão** | Métrica de priorização: candidatos que prometem reduzir incerteza valem mais |
| **Novelty Search / Quality-Diversity** (Lehman & Stanley; MAP-Elites — Mouret & Clune) | Manter **nichos diversos** supera otimizar um único objetivo | Frontier como **mapa de nichos** (frente × tema × dificuldade), melhor de cada nicho (Ciclo 4) |
| **Active learning** (Settles; 2,6× de Let's Verify) | Selecionar por valor informacional multiplica eficiência | `expected_epistemic_gain / custo` como ranking intra-frente |

## ⚠️ Q6 — Avisos honestos (o que a literatura proíbe)

| Evidência | Aviso | Regra adotada |
|---|---|---|
| **LLMs Cannot Self-Correct Reasoning Yet** (Huang et al., ICLR 2024) | Auto-correção **intrínseca** (sem feedback externo) **degrada** desempenho | O modelo nunca é o verificador final de si (invariante 2) |
| **Small LMs Need Strong Verifiers to Self-Correct** (2024, arXiv:2404.17140) | Modelos ≤13B não se autocorrigem sem verificador mais forte | No mundo weak-model, "verificador mais forte" = **executor** (testes, builds, schemas), não outro LLM |
| **Feedback Friction** (2025, arXiv:2506.11930) | LLMs incorporam mal feedback dado apenas como texto | Lesson = **regra estrutural aplicada mecanicamente** (D24), não prosa decorativa |
| **NELL** (2018) | Crenças falsas se acumulam; humanos precisaram intervir em feedback loops | F2 inclui revalidação periódica permanente; confiança é revisável |

## Lacuna honesta que o próprio estudo preenche

TinyGSM usou verificador **fine-tuned**; nós não fine-tunamos — usaremos validadores determinísticos + execução (código é verificável rodando). A literatura não prova que um runtime contínuo, auto-melhorante e weak-model end-to-end funciona — **essa é exatamente a lacuna empírica que o EON v2 se propõe a estudar**. O corpo de conhecimento citado que a F1 produz será também a evidência da tese (métrica central: 01-THESIS.md).

## Referências (com ID confirmado onde verificado)

1. Mitchell, T. et al. — *Never-Ending Learning*. CACM 2018; AAAI 2018 (paradigma).
2. Liu, Z. et al. — *TinyGSM: achieving >80% on GSM8k with small language models*. arXiv:2312.09241 (2023).
3. Brown, B. et al. — *Large Language Monkeys: Scaling Inference Compute with Repeated Sampling*. arXiv:2407.21787 (2024).
4. Lightman, H. et al. — *Let's Verify Step by Step*. arXiv:2305.20050 (2023).
5. Kambhampati, S. et al. — *LLMs Can't Plan, But Can Help Planning in LLM-Modulo Frameworks*. arXiv:2402.01817; ICML 2024.
6. Volokhova, A. et al. (Google Cloud AI Research) — *AI Parenting: Grounding LLM Agents via Self-Inquiry*. 2025. (arXiv ID a confirmar — A09)
7. Shinn, N. et al. — *Reflexion: Language Agents with Verbal Reinforcement Learning*. arXiv:2303.11366; NeurIPS 2023.
8. Zhao, A. et al. — *ExpeL: LLM Agents Are Experiential Learners*. arXiv:2308.10144; AAAI 2024.
9. Park, J.S. et al. — *Generative Agents: Interactive Simulacra of Human Behavior*. arXiv:2304.03442; UIST 2023.
10. Wang, G. et al. — *Voyager: An Open-Ended Embodied Agent with Large Language Models*. arXiv:2305.16291 (2023).
11. Lin, C. et al. — *Sleep-time Compute: Beyond Inference Scaling at Test-time*. arXiv:2504.13171 (2025).
12. Schmidhuber, J. — *A Survey of Gödel Machines*. 2003; e *Formal Theory of Creativity, Fun, and Intrinsic Motivation* (1990–2010).
13. Zhang, J. et al. — *Darwin Godel Machine: Open-Ended Evolution of Self-Improving Agents*. arXiv:2505.22954 (2025).
14. Huang, J. et al. — *Large Language Models Cannot Self-Correct Reasoning Yet*. ICLR 2024.
15. *Small Language Models Need Strong Verifiers to Self-Correct Reasoning*. arXiv:2404.17140 (2024).
16. *Feedback Friction: LLMs Struggle to Fully Incorporate External Feedback*. arXiv:2506.11930 (2025).
17. Yao, S. et al. — *τ-bench: A Benchmark for Tool-Agent-User Interaction in Real-World Domains*. 2024 (+ literatura de longa-horizonte: TheAgentCompany 2024, dados de pass^k).
18. Lehman, J. & Stanley, K. — *Abandoning Objectives: Evolution Through the Search for Novelty*. 2011; Mouret, J.-B. & Clune, J. — *Illuminating search spaces by mapping elites* (MAP-Elites), 2015.
19. Settles, B. — *Active Learning Literature Survey*. 2009.
20. Franklin, S. — *LIDA: A Computational Model of Global Workspace Theory* (ciclos cognitivos como átomos; GWT de Baars).

## Perguntas abertas

- A09: confirmar ID arXiv do AI Parenting — tarefa de pesquisa da F1 (não é decisão de design; o plano não depende dela)
