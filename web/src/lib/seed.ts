// POC data. PEC 221/2019 and PEC 8/2025 are real (Câmara and Senado open data,
// fetched 2026-10-28); summaries/comments on them are the POC team's. Everything
// else is invented.
import type {
  AuditEntry,
  Bill,
  Chamber,
  Contribution,
  Legislator,
  OfficialSource,
  User,
} from "./types";
import { PEC221_SENADO_2TURNO } from "./data/pec221-senado-2turno";

function day(offset: number): string {
  const d = new Date();
  d.setUTCHours(12, 0, 0, 0);
  d.setUTCDate(d.getUTCDate() + offset);
  return d.toISOString();
}

export const TOPICS = [
  "Saúde",
  "Educação",
  "Transporte",
  "Meio ambiente",
  "Segurança",
  "Tributos",
  "Habitação",
  "Cultura",
  "Trabalho",
];

export function seed() {
  const chambers: Chamber[] = [
    { id: "camara", name: "Câmara dos Deputados", sphere: "federal", hasApi: true },
    { id: "senado", name: "Senado Federal", sphere: "federal", hasApi: true },
    { id: "alego", name: "Assembleia Legislativa de Goiás", sphere: "state", uf: "GO", hasApi: false },
    { id: "cm-goiania", name: "Câmara Municipal de Goiânia", sphere: "municipal", uf: "GO", city: "Goiânia", hasApi: false },
    { id: "cm-anapolis", name: "Câmara Municipal de Anápolis", sphere: "municipal", uf: "GO", city: "Anápolis", hasApi: false },
  ];

  const officialSources: OfficialSource[] = [
    { domain: "camara.leg.br", chamberId: "camara" },
    { domain: "senado.leg.br", chamberId: "senado" },
    { domain: "al.go.leg.br", chamberId: "alego" },
    { domain: "goiania.go.leg.br", chamberId: "cm-goiania" },
    { domain: "camaraanapolis.com.br", chamberId: "cm-anapolis" },
  ];

  const legislators: Legislator[] = [
    // Real legislators (Câmara open data API); the rest are fictional.
    { id: "l-reginaldo", name: "Reginaldo Lopes", terms: [{ chamberId: "camara", party: "PT", uf: "MG", start: "2019-02-01", end: "2023-01-31" }, { chamberId: "camara", party: "PT", uf: "MG", start: "2023-02-01" }] },
    { id: "l-erika", name: "Erika Hilton", terms: [{ chamberId: "camara", party: "PSOL", uf: "SP", start: "2023-02-01" }] },
    { id: "l3", name: "Marta Queiroz", terms: [{ chamberId: "senado", party: "PYB", uf: "MG", start: "2023-02-01" }] },
    { id: "l4", name: "Caio Brandão", terms: [{ chamberId: "alego", party: "PXA", uf: "GO", start: "2023-02-01" }] },
    { id: "l5", name: "Lúcia Fontes", terms: [{ chamberId: "cm-goiania", party: "PZC", uf: "GO", start: "2025-01-01" }] },
    { id: "l6", name: "Tiago Mendes", terms: [{ chamberId: "cm-anapolis", party: "PXA", uf: "GO", start: "2025-01-01" }] },
  ];

  const users: User[] = [
    { id: "u-admin", name: "Ana (Admin)", email: "admin@nelisa.test", password: "nelisa123", role: "admin", active: true, mustChangePassword: false },
    { id: "u-rui", name: "Rui (Revisor)", email: "rui@nelisa.test", password: "nelisa123", role: "reviewer", active: true, mustChangePassword: false },
    { id: "u-rita", name: "Rita (Revisora)", email: "rita@nelisa.test", password: "nelisa123", role: "reviewer", active: true, mustChangePassword: false },
    { id: "u-edu", name: "Edu (Editor)", email: "edu@nelisa.test", password: "nelisa123", role: "editor", active: true, mustChangePassword: false },
  ];

  const bills: Bill[] = [
    {
      id: "pec-221-2019",
      kind: "PEC",
      chamberId: "senado",
      number: 221,
      year: 2019,
      ementa: "Altera o art. 7º da Constituição Federal para reduzir a duração máxima semanal do trabalho e prever 2 (dois) dias de repouso semanal remunerado, sem qualquer redução salarial; e dá outras providências.",
      authors: [{ kind: "legislator", legislatorId: "l-reginaldo" }],
      state: "ready",
      createdAt: "2026-05-28T12:00:00.000Z",
      version: {
        id: "v-camara-final",
        officialUrl: "https://www25.senado.leg.br/web/atividade/materias/-/materia/174386",
        createdAt: "2026-05-28T12:00:00.000Z",
        provisions: [
          { id: "ementa", label: "Ementa", text: "Altera o art. 7º da Constituição Federal para reduzir a duração máxima semanal do trabalho e prever 2 (dois) dias de repouso semanal remunerado, sem qualquer redução salarial; e dá outras providências." },
          { id: "a1-xiii", label: "Art. 1º (CF, art. 7º, XIII)", text: "XIII - duração do trabalho normal não superior a 8 (oito) horas diárias e 40 (quarenta) horas semanais, facultada a compensação de horários e a redução da jornada, mediante acordo ou convenção coletiva de trabalho;" },
          { id: "a1-xv", label: "Art. 1º (CF, art. 7º, XV)", text: "XV – repouso semanal remunerado de 2 (dois) dias, um dos quais preferencialmente aos domingos;" },
          { id: "a1-p2", label: "Art. 1º (CF, art. 7º, § 2º)", text: "§ 2º Excepcionalmente, convenção ou acordo coletivo de trabalho poderão, inclusive para os trabalhadores sujeitos a regimes diferenciados de trabalho estabelecidos em lei ou norma regulamentadora, estabelecer regime compensatório que assegure, na média, 2 (dois) dias de repouso semanal remunerado dentro do mês-calendário, garantido o gozo de pelo menos 1 (um) dos dias dentro do período máximo de uma semana de trabalho." },
          { id: "a1-p3", label: "Art. 1º (CF, art. 7º, § 3º)", text: "§ 3º Lei poderá dispor sobre hipóteses e condições em que a duração do trabalho e os dias de repouso semanal remunerado poderão observar regimes diferenciados, respeitados os limites previstos nos incisos XIII, XIV e XV do caput deste artigo." },
          { id: "a2", label: "Art. 2º", text: "A diminuição da duração do trabalho normal e o incremento do repouso semanal remunerado em cumprimento a esta Emenda Constitucional aplicam-se aos contratos de trabalho em vigor e serão implementados sem qualquer redução salarial nominal, proporcional ou de qualquer outra espécie." },
          { id: "a2-pu", label: "Art. 2º, parágrafo único", text: "A irredutibilidade salarial estabelecida no caput deste artigo aplica-se inclusive aos pisos salariais." },
          { id: "a3", label: "Art. 3º", text: "Decorridos 2 (dois) meses da publicação desta Emenda Constitucional, ficarão sem efeito as cláusulas de convenções e acordos coletivos de trabalho sobre duração do trabalho e do repouso semanal remunerado incompatíveis com as disposições desta Emenda Constitucional." },
          { id: "a4", label: "Art. 4º", text: "A entrada em vigor desta Emenda Constitucional não implicará redução proporcional das jornadas de trabalho já fixadas em patamar igual ou inferior a 40 (quarenta) horas semanais, sem prejuízo da aplicação do disposto no inciso XV do caput do art. 7º da Constituição Federal." },
          { id: "a5", label: "Art. 5º", text: "Lei complementar poderá estabelecer medidas transitórias, condicionadas à manutenção de níveis de emprego, de mitigação dos impactos decorrentes desta Emenda Constitucional, para os microempreendedores individuais, as microempresas e as empresas de pequeno porte." },
          { id: "a6", label: "Art. 6º", text: "A redução da duração do trabalho normal para 40 (quarenta) horas semanais será implementada de forma progressiva, observado o seguinte:" },
          { id: "a6-i", label: "Art. 6º, I", text: "I - 2 (dois) meses após a publicação desta Emenda Constitucional, a duração do trabalho normal não excederá a 42 (quarenta e duas) horas semanais;" },
          { id: "a6-ii", label: "Art. 6º, II", text: "II - 1 (um) ano após o decurso do prazo de que trata o inciso I deste artigo, a duração do trabalho normal não excederá a 40 (quarenta) horas semanais." },
          { id: "a7", label: "Art. 7º", text: "No prazo previsto no inciso II do caput do art. 6º, convenção ou acordo coletivo de trabalho poderão ampliar a duração diária do trabalho normal para viabilizar a distribuição da duração semanal do trabalho, estabelecida no inciso I do caput do art. 6º desta Emenda Constitucional, respeitado o disposto no inciso XV do caput do art. 7º da Constituição Federal." },
          { id: "a8", label: "Art. 8º", text: "Ao empregado portador de diploma de nível superior e que perceba remuneração mensal igual ou superior a duas vezes e meia o limite máximo dos benefícios do Regime Geral de Previdência Social não se aplicam as regras relativas à duração do trabalho e ao controle da jornada, salvo por liberalidade do empregador ou se houver previsão em acordo ou convenção coletiva de trabalho, respeitado o inciso XV e § 2º do art. 7º da Constituição Federal." },
          { id: "a8-p1", label: "Art. 8º, § 1º", text: "O disposto no caput deste artigo não se aplica aos empregados públicos da administração direta e indireta de quaisquer dos Poderes da União, dos Estados, do Distrito Federal e dos Municípios." },
          { id: "a8-p2", label: "Art. 8º, § 2º", text: "Compete à Justiça do Trabalho processar e julgar as ações relativas ao disposto no caput deste artigo." },
          { id: "a9", label: "Art. 9º", text: "Nos contratos celebrados pela administração pública direta e indireta da União, dos Estados, do Distrito Federal e dos Municípios, vigentes na data de publicação desta Emenda Constitucional e cuja execução envolva emprego direto de mão de obra, as disposições relativas à redução da duração do trabalho normal serão aplicadas após aditamento contratual para manutenção do equilíbrio econômico-financeiro, conforme o regime jurídico aplicável, a ser formalizado no prazo máximo de 1 (um) ano, contado da publicação desta Emenda Constitucional." },
          { id: "a9-p1", label: "Art. 9º, § 1º", text: "O disposto neste artigo aplica-se aos contratos regidos pela legislação de licitações e contratos administrativos, de concessões e permissões de serviços e obras públicas, de parcerias público-privadas e de outros instrumentos de colaboração com a iniciativa privada." },
          { id: "a9-p2", label: "Art. 9º, § 2º", text: "Os empregados alocados na execução dos contratos de que trata este artigo passam a ser abrangidos pelas disposições desta Emenda Constitucional relativas à duração do trabalho normal na data da formalização do aditamento ou, independentemente deste, ao término do prazo previsto no caput deste artigo, assegurada a irredutibilidade salarial." },
          { id: "a9-p3", label: "Art. 9º, § 3º", text: "Os contratos aditados no prazo de 2 (dois) meses da data de publicação desta Emenda Constitucional deverão observar as disposições sobre redução da duração do trabalho normal e incremento do repouso semanal remunerado a partir do respectivo início das vigências instituídas nesta Emenda Constitucional." },
          { id: "a10", label: "Art. 10", text: "Esta Emenda Constitucional entra em vigor:" },
          { id: "a10-i", label: "Art. 10, I", text: "I - em 2 (dois) meses de sua publicação, em relação à alteração promovida pelo art. 1º desta Emenda Constitucional no inciso XV do caput do art. 7º da Constituição Federal;" },
          { id: "a10-ii", label: "Art. 10, II", text: "II - na data da sua publicação, em relação aos demais dispositivos." },
        ],
        comments: [
          { id: "c1", provisionId: "a1-xiii", text: "Antes: até 44 horas semanais. Agora: até 40, mantido o limite de 8 horas por dia.", authorId: "u-edu" },
          { id: "c2", provisionId: "a1-xv", text: "Antes: repouso semanal de 1 dia. É esta mudança que encerra a escala 6x1 (seis dias de trabalho para um de folga).", authorId: "u-edu" },
          { id: "c3", provisionId: "a1-p2", text: "Exceção: por acordo coletivo, os 2 dias de folga podem ser contados na média do mês, desde que haja ao menos 1 folga por semana.", authorId: "u-edu" },
          { id: "c4", provisionId: "a6", text: "A redução é em duas etapas: 42 horas dois meses após a publicação e 40 horas um ano depois disso.", authorId: "u-edu" },
          { id: "c5", provisionId: "a8", text: "Cria uma exceção: quem tem diploma de nível superior e ganha pelo menos 2,5 vezes o teto do INSS fica fora das regras de jornada e controle de ponto, salvo acordo em contrário. A folga de 2 dias continua valendo para esse grupo.", authorId: "u-edu" },
          { id: "c6", provisionId: "a10", text: "Datas diferentes: os 2 dias de repouso valem 2 meses após a publicação; os demais dispositivos, na data da publicação. A jornada de 40 horas segue o cronograma do Art. 6º.", authorId: "u-edu" },
        ],
      },
      summary: "A emenda reduz a jornada máxima de trabalho de 44 para 40 horas semanais e garante 2 dias de repouso remunerado por semana, um deles preferencialmente no domingo, o que encerra a escala 6x1. O salário não pode ser reduzido por causa disso, inclusive pisos salariais. A redução é gradual: 42 horas dois meses após a publicação e 40 horas um ano depois. Acordos coletivos podem distribuir as folgas na média do mês, com ao menos uma por semana. Há uma exceção para quem tem diploma de nível superior e ganha pelo menos 2,5 vezes o teto do INSS, que fica fora das regras de jornada, mas mantém a folga de 2 dias. Micro e pequenas empresas podem ter medidas de transição por lei complementar, e contratos com o poder público têm até um ano para se ajustar.",
      shortSummary: "Fim da escala 6x1: jornada máxima cai de 44 para 40 horas por semana, de forma gradual, com 2 dias de folga semanal e sem redução de salário.",
      topics: ["Trabalho"],
      summaryAuthorIds: ["u-edu"],
      approvals: [{ userId: "u-rui", at: "2026-10-20T12:00:00.000Z", bypass: false }],
      pendingSecondReview: false,
      progress: [
        { id: "p1", date: "2019-12-11T12:00:00.000Z", kind: "presented", description: "Apresentada na Câmara dos Deputados" },
        { id: "p2", date: "2026-05-27T12:00:00.000Z", kind: "voted", description: "Aprovada no Plenário da Câmara dos Deputados" },
        { id: "p3", date: "2026-05-28T12:00:00.000Z", kind: "presented", description: "Recebida pelo Senado Federal (Casa revisora)" },
        { id: "p4", date: "2026-09-02T12:00:00.000Z", kind: "voted", description: "Parecer aprovado na Comissão de Constituição e Justiça (CCJ)" },
        { id: "p5", date: "2026-10-14T12:00:00.000Z", kind: "voted", description: "Aprovada no Plenário do Senado em 1º e 2º turnos (Sim: 75; Não: 1)" },
        { id: "p6", date: "2026-10-16T12:00:00.000Z", kind: "promulgated", description: "Promulgada como Emenda Constitucional nº 141/2026 (DOU, edição extra)" },
        { id: "p7", date: "2026-10-16T12:00:00.000Z", kind: "in_force", description: "Em vigor, exceto o repouso semanal de 2 dias (Art. 10, II)" },
        { id: "p8", date: "2026-12-16T12:00:00.000Z", kind: "in_force", description: "Passa a valer o repouso semanal de 2 dias (Art. 10, I)" },
      ],
      rollCall: { description: "Plenário do Senado, 2º turno, 14/10/2026", votes: PEC221_SENADO_2TURNO },
      internalNotes: [{ id: "n1", authorId: "u-rui", at: "2026-10-20T12:00:00.000Z", text: "Na Câmara ela é a mesma PEC 221/2019, com outra tramitação. Como juntar as duas Casas?" }],
      aiDraftUsed: false,
    },
    {
      id: "b2",
      kind: "PL",
      chamberId: "senado",
      number: 2002,
      year: 2026,
      ementa: "Altera regras de licenciamento ambiental para obras de saneamento básico.",
      authors: [{ kind: "legislator", legislatorId: "l3" }],
      state: "in_review",
      createdAt: day(-20),
      version: {
        id: "v1",
        officialUrl: "https://www25.senado.leg.br/exemplo/pl-2002-2026",
        createdAt: day(-20),
        provisions: [
          { id: "ementa", label: "Ementa", text: "Altera regras de licenciamento ambiental para obras de saneamento básico." },
          { id: "a1", label: "Art. 1º", text: "As obras de saneamento básico de pequeno porte ficam sujeitas a licenciamento ambiental simplificado." },
          { id: "a2", label: "Art. 2º", text: "Regulamento definirá os critérios de pequeno porte." },
          { id: "a3", label: "Art. 3º", text: "Esta Lei entra em vigor na data de sua publicação." },
        ],
        comments: [],
      },
      summary: "",
      shortSummary: "",
      topics: [],
      summaryAuthorIds: [],
      approvals: [],
      pendingSecondReview: false,
      progress: [
        { id: "p1", date: day(-20), kind: "presented", description: "Apresentado" },
        { id: "p2", date: day(2), kind: "vote_scheduled", description: "Votação na Comissão de Meio Ambiente" },
      ],
      internalNotes: [{ id: "n1", authorId: "u-rui", at: day(-1), text: "O Art. 2º deixa tudo para o regulamento. Vale um comentário?" }],
      aiDraftUsed: false,
    },
    {
      id: "pec-8-2025",
      kind: "PEC",
      chamberId: "camara",
      number: 8,
      year: 2025,
      ementa: "Dá nova redação ao inciso XIII, do artigo 7° da Constituição Federal para dispor sobre a redução da jornada de trabalho para quatro dias por semana no Brasil.",
      authors: [{ kind: "legislator", legislatorId: "l-erika" }, { kind: "cosigners", count: 225 }],
      state: "in_review",
      createdAt: "2025-02-25T12:00:00.000Z",
      version: {
        id: "v1",
        officialUrl: "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2485341",
        createdAt: "2025-02-25T12:00:00.000Z",
        provisions: [
          { id: "ementa", label: "Ementa", text: "Dá nova redação ao inciso XIII, do artigo 7° da Constituição Federal para dispor sobre a redução da jornada de trabalho para quatro dias por semana no Brasil." },
          { id: "a1", label: "Art. 1º (CF, art. 7º, XIII)", text: "XIII – duração do trabalho normal não superior a oito horas diárias e trinta e seis horas semanais, com jornada de trabalho de quatro dias por semana, facultada a compensação de horários e a redução de jornada, mediante acordo ou convenção coletiva de trabalho;" },
          { id: "a2", label: "Art. 2º", text: "Esta Emenda Constitucional entra em vigor 360 dias após a data da sua publicação." },
          { id: "just", label: "Justificativa (trecho)", text: "Esta emenda à Constituição surge a partir das demandas e reivindicações dos trabalhadores, por meio de mecanismos participativos, como a petição pública online do Movimento “Vida Além do Trabalho” [...] em que quase 800 mil brasileiros e brasileiras cobram do Congresso Nacional o fim da jornada 6x1 e adoção da jornada de trabalho de 4 dias na semana." },
        ],
        comments: [],
      },
      summary: "",
      shortSummary: "",
      topics: [],
      summaryAuthorIds: [],
      approvals: [],
      pendingSecondReview: false,
      progress: [
        { id: "p1", date: "2025-02-25T12:00:00.000Z", kind: "presented", description: "Apresentada no Plenário da Câmara" },
        { id: "p2", date: "2026-02-09T12:00:00.000Z", kind: "attached", description: "Apensada à PEC 221/2019 (passa a tramitar junto com ela)" },
        { id: "p3", date: "2026-05-27T12:00:00.000Z", kind: "archived", description: "Declarada prejudicada pela aprovação da PEC 221/2019 no Plenário; arquivada" },
      ],
      internalNotes: [{ id: "n1", authorId: "u-rita", at: "2026-10-27T12:00:00.000Z", text: "Foi prejudicada pela PEC 221/2019. Vale revisar uma proposição arquivada?" }],
      aiDraftUsed: false,
    },
    {
      id: "b4",
      kind: "PL",
      chamberId: "alego",
      number: 301,
      year: 2026,
      ementa: "Proíbe a cobrança de taxa de religação de energia em dias de chuva intensa.",
      authors: [{ kind: "legislator", legislatorId: "l4" }],
      state: "in_review",
      createdAt: day(-15),
      version: {
        id: "v1",
        officialUrl: "https://al.go.leg.br/exemplo/pl-301-2026",
        createdAt: day(-15),
        provisions: [
          { id: "ementa", label: "Ementa", text: "Proíbe a cobrança de taxa de religação de energia em dias de chuva intensa." },
          { id: "a1", label: "Art. 1º", text: "É vedada a cobrança de taxa de religação quando a interrupção ocorrer em dia de alerta meteorológico." },
          { id: "a2", label: "Art. 2º", text: "Esta Lei entra em vigor na data de sua publicação." },
        ],
        comments: [],
      },
      summary: "",
      shortSummary: "",
      topics: [],
      summaryAuthorIds: [],
      approvals: [],
      pendingSecondReview: false,
      progress: [{ id: "p1", date: day(-15), kind: "presented", description: "Lido em Plenário" }],
      claim: { userId: "u-edu", lastActivity: day(-1) },
      internalNotes: [],
      aiDraftUsed: false,
    },
    {
      id: "b5",
      kind: "PL",
      chamberId: "cm-goiania",
      number: 45,
      year: 2026,
      ementa: "Cria hortas comunitárias em praças públicas do município.",
      authors: [{ kind: "legislator", legislatorId: "l5" }],
      state: "draft",
      createdAt: day(-3),
      version: {
        id: "v1",
        officialUrl: "https://www.goiania.go.leg.br/exemplo/pl-45-2026",
        createdAt: day(-3),
        provisions: [
          { id: "ementa", label: "Ementa", text: "Cria hortas comunitárias em praças públicas do município." },
          { id: "a1", label: "Art. 1º", text: "O Poder Executivo poderá ceder áreas de praças públicas para hortas comunitárias." },
          { id: "a2", label: "Art. 2º", text: "Esta Lei entra em vigor na data de sua publicação." },
        ],
        comments: [],
      },
      summary: "",
      shortSummary: "",
      topics: [],
      summaryAuthorIds: [],
      approvals: [],
      pendingSecondReview: false,
      progress: [
        { id: "p1", date: day(-3), kind: "presented", description: "Protocolado" },
        { id: "p2", date: day(1), kind: "vote_scheduled", description: "Primeira votação em Plenário" },
      ],
      internalNotes: [],
      aiDraftUsed: false,
    },
    {
      id: "b6",
      kind: "PL",
      chamberId: "cm-anapolis",
      number: 12,
      year: 2026,
      ementa: "Institui a Semana Municipal da Cultura Popular.",
      authors: [{ kind: "body", name: "Prefeitura de Anápolis (Executivo)" }],
      state: "ready",
      createdAt: day(-60),
      version: {
        id: "v1",
        officialUrl: "https://www.camaraanapolis.com.br/exemplo/pl-12-2026",
        createdAt: day(-60),
        provisions: [
          { id: "ementa", label: "Ementa", text: "Institui a Semana Municipal da Cultura Popular." },
          { id: "a1", label: "Art. 1º", text: "Fica instituída a Semana Municipal da Cultura Popular, a ser realizada anualmente na última semana de agosto." },
          { id: "a2", label: "Art. 2º", text: "Esta Lei entra em vigor na data de sua publicação." },
        ],
        comments: [],
      },
      summary: "Cria no calendário oficial de Anápolis uma semana anual dedicada à cultura popular, na última semana de agosto. O texto não prevê gastos.",
      shortSummary: "Anápolis ganha uma Semana da Cultura Popular no calendário oficial, na última semana de agosto, sem gasto previsto.",
      topics: ["Cultura"],
      summaryAuthorIds: ["u-admin"],
      approvals: [{ userId: "u-admin", at: day(-20), bypass: true }],
      pendingSecondReview: true,
      progress: [
        { id: "p1", date: day(-60), kind: "presented", description: "Mensagem do Executivo lida em Plenário" },
        { id: "p2", date: day(-25), kind: "voted", description: "Aprovado em votação simbólica" },
        { id: "p3", date: day(-18), kind: "sanctioned", description: "Sancionado" },
        { id: "p4", date: day(-17), kind: "in_force", description: "Em vigor" },
      ],
      internalNotes: [],
      aiDraftUsed: false,
    },
  ];

  const contributions: Contribution[] = [
    { trackingCode: "NL-7K2Q", kind: "progress_update", status: "pending", createdAt: day(-1), billId: "b5", progressKind: "vote_scheduled", date: day(8), source: "https://www.goiania.go.leg.br/exemplo/pauta", note: "A votação foi adiada" },
    { trackingCode: "NL-3MZP", kind: "referral", status: "pending", createdAt: day(-2), url: "https://al.go.leg.br/exemplo/pl-410-2026", source: "https://al.go.leg.br/exemplo/pl-410-2026" },
  ];

  const audit: AuditEntry[] = [
    { at: day(-20), userId: "u-admin", action: "approve", detail: "PL 12/2026 (Câmara Municipal de Anápolis) aprovado SEM segunda revisão" },
    { at: "2026-10-20T12:00:00.000Z", userId: "u-rui", action: "approve", detail: "PEC 221/2019 aprovada" },
  ];

  return { chambers, officialSources, legislators, users, bills, contributions, audit };
}
