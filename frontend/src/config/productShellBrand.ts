export const PRODUCT_SHELL_BRAND_SCHEMA = 'ProductShellBrandV1'

export const productShellBrand = {
  schemaVersion: PRODUCT_SHELL_BRAND_SCHEMA,
  name: '环枢·零售智能体',
  shortName: '环枢',
  tokens: {
    paper: '#f7f2e9',
    ink: '#132d2d',
    teal: '#147b76',
    risk: '#c65b32',
  },
} as const

type LoginCopy = {
  eyebrow: string
  headline: string
  description: string
  employeeAssistant: string
  operatingAnalysis: string
}

export type RetailAgentHomeCopy = {
  eyebrow: string
  headline: string
  compactHeadline: string
  description: string
  currentWork: string
  employeeDescription: string
  analysisDescription: string
  startWork: string
  enterWork: string
  continueWork: string
  employeeRecentWork: string
  analysisRecentWork: string
  noEmployeeRecentWork: string
  noAnalysisRecentWork: string
  handoffHint: string
  contactAdmin: string
  serviceUnavailable: string
  loopTitle: string
  loopDescription: string
  loopSteps: [string, string, string, string]
  roadmapTitle: string
  roadmapDescription: string
  current: string
  planned: string
  procurementOrdering: string
}

const loginCopyByLocale: Record<string, LoginCopy> = {
  'zh-CN': {
    eyebrow: '面向零售企业的智能体平台',
    headline: '让智能体真正进入零售经营现场。',
    description: '连接企业知识与业务数据，为零售组织提供可信、实用的智能支持。',
    employeeAssistant: '员工助理',
    operatingAnalysis: '经营分析',
  },
  'en-US': {
    eyebrow: 'An agent platform for retail enterprises',
    headline: 'Bring agents into real retail operations.',
    description: 'Connect enterprise knowledge and business data to provide trusted, practical intelligence for retail organizations.',
    employeeAssistant: 'Employee Assistant',
    operatingAnalysis: 'Operating Analysis',
  },
  'ru-RU': {
    eyebrow: 'Платформа интеллектуальных агентов для предприятий розничной торговли',
    headline: 'Интеллектуальные агенты в реальных процессах розничной торговли.',
    description: 'Объединяем знания предприятия и бизнес-данные, чтобы предоставлять розничным организациям надёжную и практичную интеллектуальную поддержку.',
    employeeAssistant: 'Помощник сотрудника',
    operatingAnalysis: 'Операционный анализ',
  },
  'ko-KR': {
    eyebrow: '리테일 기업을 위한 에이전트 플랫폼',
    headline: '에이전트를 실제 리테일 운영 현장으로 연결합니다.',
    description: '기업 지식과 비즈니스 데이터를 연결해 리테일 조직에 신뢰할 수 있고 실용적인 지능형 지원을 제공합니다.',
    employeeAssistant: '직원 지원',
    operatingAnalysis: '운영 분석',
  },
}

export function getProductShellLoginCopy(locale: string): LoginCopy {
  return loginCopyByLocale[locale] || loginCopyByLocale['zh-CN']
}

const homeCopyByLocale: Record<string, RetailAgentHomeCopy> = {
  'zh-CN': {
    eyebrow: '环枢·零售智能体',
    headline: '让企业知识与经营数据，围绕同一项工作继续推进。',
    compactHeadline: '同一项工作，从知识走向经营判断。',
    description: '先向员工助理了解制度、流程和业务背景；需要核验经营数据时，带着原问题进入经营分析。',
    currentWork: '当前工作空间',
    employeeDescription: '查询企业制度、流程和业务知识，处理日常问题。',
    analysisDescription: '基于受治理经营数据看指标、找异常，获得有依据的分析。',
    startWork: '打开员工助理',
    enterWork: '进入经营分析',
    continueWork: '继续最近对话',
    employeeRecentWork: '最近对话',
    analysisRecentWork: '最近分析',
    noEmployeeRecentWork: '还没有员工助理对话',
    noAnalysisRecentWork: '还没有经营分析',
    handoffHint: '需要核验经营数据时，可把员工助理中的原问题带到经营分析。',
    contactAdmin: '请联系企业管理员开通经营分析权限',
    serviceUnavailable: '经营分析服务暂不可用',
    loopTitle: '两个工作空间，一条连续路径',
    loopDescription: '员工助理负责企业知识，经营分析负责受治理数据；两个工作空间各自保留历史和依据。',
    loopSteps: ['了解业务背景', '查清制度流程', '核验经营数据', '回到经营行动'],
    roadmapTitle: '能力开放说明',
    roadmapDescription: '当前开放员工助理与经营分析；采购与订货等执行协同能力只在完成真实业务接入后开放。',
    current: '当前可用',
    planned: '待业务接入',
    procurementOrdering: '采购与订货',
  },
  'en-US': {
    eyebrow: 'HuanShu Retail Agent',
    headline: 'Move the same task forward with enterprise knowledge and operating data.',
    compactHeadline: 'One task, from knowledge to operating judgment.',
    description: 'Start with the Employee Assistant for policies, processes, and context. When data needs verification, carry the original question into Operating Analysis.',
    currentWork: 'Current workspaces',
    employeeDescription: 'Find enterprise policies, processes, and business knowledge for everyday work.',
    analysisDescription: 'Use governed operating data to inspect metrics, find anomalies, and reach evidence-based conclusions.',
    startWork: 'Open Employee Assistant',
    enterWork: 'Open operating analysis',
    continueWork: 'Continue recent conversation',
    employeeRecentWork: 'Recent conversation',
    analysisRecentWork: 'Recent analysis',
    noEmployeeRecentWork: 'No Employee Assistant conversations yet',
    noAnalysisRecentWork: 'No operating analysis yet',
    handoffHint: 'When operating data needs verification, carry the original Employee Assistant question into Operating Analysis.',
    contactAdmin: 'Contact an enterprise administrator for operating analysis access',
    serviceUnavailable: 'Operating analysis is temporarily unavailable',
    loopTitle: 'Two workspaces, one continuous path',
    loopDescription: 'Employee Assistant owns enterprise knowledge; Operating Analysis owns governed data. Each workspace keeps its own history and evidence.',
    loopSteps: ['Understand context', 'Clarify policies', 'Verify operating data', 'Return to action'],
    roadmapTitle: 'Capability availability',
    roadmapDescription: 'Employee Assistant and Operating Analysis are available now. Execution workflows such as procurement and ordering open only after real business integration.',
    current: 'Available now',
    planned: 'Awaiting integration',
    procurementOrdering: 'Procurement and ordering',
  },
  'ru-RU': {
    eyebrow: 'Розничный агент HuanShu',
    headline: 'Продвигайте одну задачу с помощью корпоративных знаний и операционных данных.',
    compactHeadline: 'Одна задача: от знаний к операционному решению.',
    description: 'Сначала уточните правила, процессы и контекст у помощника сотрудника. Если нужны данные, перенесите исходный вопрос в операционный анализ.',
    currentWork: 'Рабочие пространства',
    employeeDescription: 'Находите корпоративные правила, процессы и знания для повседневной работы.',
    analysisDescription: 'Проверяйте показатели и отклонения по управляемым операционным данным.',
    startWork: 'Открыть помощника',
    enterWork: 'Открыть операционный анализ',
    continueWork: 'Продолжить последний диалог',
    employeeRecentWork: 'Последний диалог',
    analysisRecentWork: 'Последний анализ',
    noEmployeeRecentWork: 'Диалогов с помощником пока нет',
    noAnalysisRecentWork: 'Операционного анализа пока нет',
    handoffHint: 'Когда нужны операционные данные, перенесите исходный вопрос из помощника сотрудника в операционный анализ.',
    contactAdmin: 'Обратитесь к администратору предприятия для получения доступа',
    serviceUnavailable: 'Операционный анализ временно недоступен',
    loopTitle: 'Два пространства, один непрерывный путь',
    loopDescription: 'Помощник отвечает за корпоративные знания, операционный анализ — за управляемые данные. История и основания сохраняются отдельно.',
    loopSteps: ['Понять контекст', 'Уточнить правила', 'Проверить данные', 'Вернуться к действию'],
    roadmapTitle: 'Доступность возможностей',
    roadmapDescription: 'Помощник и операционный анализ доступны сейчас. Исполнительные процессы откроются после реальной интеграции с бизнесом.',
    current: 'Доступно сейчас',
    planned: 'Ожидает интеграции',
    procurementOrdering: 'Закупки и заказы',
  },
  'ko-KR': {
    eyebrow: 'HuanShu 리테일 에이전트',
    headline: '기업 지식과 운영 데이터로 같은 업무를 계속 추진합니다.',
    compactHeadline: '하나의 업무, 지식에서 운영 판단까지.',
    description: '직원 도우미에서 규정, 절차, 업무 맥락을 먼저 확인하고 데이터 검증이 필요하면 원래 질문을 운영 분석으로 이어갑니다.',
    currentWork: '현재 작업 공간',
    employeeDescription: '기업 규정, 절차, 업무 지식을 찾아 일상 업무를 처리합니다.',
    analysisDescription: '거버넌스가 적용된 운영 데이터로 지표와 이상을 확인해 근거 있는 분석을 얻습니다.',
    startWork: '직원 도우미 열기',
    enterWork: '운영 분석 열기',
    continueWork: '최근 대화 계속하기',
    employeeRecentWork: '최근 대화',
    analysisRecentWork: '최근 분석',
    noEmployeeRecentWork: '직원 도우미 대화가 없습니다',
    noAnalysisRecentWork: '운영 분석이 없습니다',
    handoffHint: '운영 데이터 검증이 필요하면 직원 도우미의 원래 질문을 운영 분석으로 이어갈 수 있습니다.',
    contactAdmin: '운영 분석 권한은 기업 관리자에게 문의하세요',
    serviceUnavailable: '운영 분석 서비스를 일시적으로 사용할 수 없습니다',
    loopTitle: '두 작업 공간, 하나의 연속된 흐름',
    loopDescription: '직원 도우미는 기업 지식, 운영 분석은 거버넌스 데이터를 담당하며 각 작업 공간의 이력과 근거는 별도로 유지됩니다.',
    loopSteps: ['업무 맥락 이해', '규정과 절차 확인', '운영 데이터 검증', '현장 실행으로 복귀'],
    roadmapTitle: '기능 제공 현황',
    roadmapDescription: '직원 도우미와 운영 분석은 현재 제공됩니다. 구매·발주 등 실행 기능은 실제 업무 연동이 완료된 후에만 제공됩니다.',
    current: '현재 사용 가능',
    planned: '업무 연동 대기',
    procurementOrdering: '구매 및 발주',
  },
}

export function getRetailAgentHomeCopy(locale: string): RetailAgentHomeCopy {
  return homeCopyByLocale[locale] || homeCopyByLocale['zh-CN']
}

export type EnterpriseAdministrationCopy = {
  eyebrow: string
  title: string
  description: string
  serviceLevel: string
  serviceStatus: string
  members: string
  storage: string
  security: string
  healthy: string
  attention: string
  unavailable: string
  unknown: string
  queueTitle: string
  queueDescription: string
  queueEmpty: string
  serviceHealthTitle: string
  serviceHealthDescription: string
  serviceHealthAction: string
  retry: string
  loadFailed: string
  itemCopy: Record<string, { title: string; description: string; action: string }>
}

const enterpriseAdministrationCopyByLocale: Record<string, EnterpriseAdministrationCopy> = {
  'zh-CN': {
    eyebrow: '企业管理',
    title: '今天需要关注的企业事项',
    description: '集中查看成员、知识与服务状态；具体模型和基础设施由平台统一管理。',
    serviceLevel: '服务级别',
    serviceStatus: '服务状态',
    members: '活跃成员',
    storage: '知识存储',
    security: '安全动态',
    healthy: '运行正常',
    attention: '需要关注',
    unavailable: '暂不可用',
    unknown: '待确认',
    queueTitle: '待处理事项',
    queueDescription: '按影响程度排列，只展示当前角色可以处理的事项。',
    queueEmpty: '当前没有需要处理的事项',
    serviceHealthTitle: '平台服务支持',
    serviceHealthDescription: '企业侧仅展示可用状态。服务能力、模型与基础设施由环枢平台统一维护。',
    serviceHealthAction: '联系平台方',
    retry: '重新加载',
    loadFailed: '企业管理信息暂时无法加载',
    itemCopy: {
      license_or_capability_attention: { title: '服务能力需要处理', description: '当前企业服务配置需要平台方检查。', action: '查看服务状态' },
      edge_node_offline: { title: '边缘节点连接离线', description: '部分企业侧边缘节点当前未连接平台。', action: '查看服务状态' },
      operating_analysis_access_gap: { title: '经营分析授权待完善', description: '部分活跃成员尚未获得经营分析权限。', action: '管理成员' },
      pending_invitations: { title: '成员邀请待接受', description: '已发送的成员邀请尚未完成。', action: '管理成员' },
      knowledge_processing_failed: { title: '知识内容处理失败', description: '部分知识内容需要重新处理。', action: '查看知识库' },
      recent_high_risk_operations: { title: '近期重要权限变更', description: '最近 7 天发生了需要复核的重要操作。', action: '查看审计记录' },
    },
  },
  'en-US': {
    eyebrow: 'Enterprise administration',
    title: 'What needs attention today',
    description: 'Review members, knowledge, and service status in one place. Models and infrastructure are managed by the platform.',
    serviceLevel: 'Service level',
    serviceStatus: 'Service status',
    members: 'Active members',
    storage: 'Knowledge storage',
    security: 'Security activity',
    healthy: 'Healthy',
    attention: 'Needs attention',
    unavailable: 'Unavailable',
    unknown: 'To be confirmed',
    queueTitle: 'Action queue',
    queueDescription: 'Sorted by impact and limited to items your role can handle.',
    queueEmpty: 'Nothing needs attention right now',
    serviceHealthTitle: 'Platform support',
    serviceHealthDescription: 'Enterprise users see availability only. The HuanShu platform manages service capabilities, models, and infrastructure.',
    serviceHealthAction: 'Contact platform support',
    retry: 'Try again',
    loadFailed: 'Enterprise administration is temporarily unavailable',
    itemCopy: {
      license_or_capability_attention: { title: 'Service capability needs attention', description: 'The platform needs to review this enterprise service configuration.', action: 'View service status' },
      edge_node_offline: { title: 'Edge node offline', description: 'One or more enterprise edge nodes are not connected to the platform.', action: 'View service status' },
      operating_analysis_access_gap: { title: 'Operating analysis access incomplete', description: 'Some active members do not yet have operating analysis access.', action: 'Manage members' },
      pending_invitations: { title: 'Member invitations pending', description: 'Sent invitations have not yet been accepted.', action: 'Manage members' },
      knowledge_processing_failed: { title: 'Knowledge processing failed', description: 'Some knowledge content needs to be processed again.', action: 'Open knowledge bases' },
      recent_high_risk_operations: { title: 'Recent important permission changes', description: 'Important operations from the last 7 days should be reviewed.', action: 'View audit log' },
    },
  },
  'ru-RU': {
    eyebrow: 'Управление предприятием',
    title: 'Что требует внимания сегодня',
    description: 'Участники, знания и состояние сервиса в одном месте. Модели и инфраструктуру обслуживает платформа.',
    serviceLevel: 'Уровень сервиса', serviceStatus: 'Состояние сервиса', members: 'Активные участники', storage: 'Хранилище знаний', security: 'События безопасности',
    healthy: 'Работает нормально', attention: 'Требует внимания', unavailable: 'Недоступно', unknown: 'Нужно уточнить',
    queueTitle: 'Задачи', queueDescription: 'Сортировка по влиянию; показаны только доступные вашей роли задачи.', queueEmpty: 'Сейчас нет задач, требующих внимания',
    serviceHealthTitle: 'Поддержка платформы', serviceHealthDescription: 'Предприятие видит только доступность. Возможности, модели и инфраструктуру обслуживает платформа HuanShu.', serviceHealthAction: 'Связаться с платформой',
    retry: 'Повторить', loadFailed: 'Управление предприятием временно недоступно',
    itemCopy: {
      license_or_capability_attention: { title: 'Требуется проверка сервиса', description: 'Платформе нужно проверить конфигурацию сервиса предприятия.', action: 'Состояние сервиса' },
      edge_node_offline: { title: 'Пограничный узел не в сети', description: 'Один или несколько пограничных узлов не подключены к платформе.', action: 'Состояние сервиса' },
      operating_analysis_access_gap: { title: 'Не всем выдан доступ к анализу', description: 'У части активных участников нет доступа к операционному анализу.', action: 'Управлять участниками' },
      pending_invitations: { title: 'Ожидающие приглашения', description: 'Отправленные приглашения еще не приняты.', action: 'Управлять участниками' },
      knowledge_processing_failed: { title: 'Ошибка обработки знаний', description: 'Некоторые материалы нужно обработать повторно.', action: 'Открыть базы знаний' },
      recent_high_risk_operations: { title: 'Важные изменения прав', description: 'Следует проверить важные операции за последние 7 дней.', action: 'Открыть аудит' },
    },
  },
  'ko-KR': {
    eyebrow: '기업 관리',
    title: '오늘 확인할 기업 운영 항목',
    description: '구성원, 지식, 서비스 상태를 한곳에서 확인합니다. 모델과 인프라는 플랫폼에서 통합 관리합니다.',
    serviceLevel: '서비스 수준', serviceStatus: '서비스 상태', members: '활성 구성원', storage: '지식 저장공간', security: '보안 활동',
    healthy: '정상 운영', attention: '확인 필요', unavailable: '사용 불가', unknown: '확인 대기',
    queueTitle: '처리할 항목', queueDescription: '영향도 순으로 현재 역할이 처리할 수 있는 항목만 표시합니다.', queueEmpty: '현재 처리할 항목이 없습니다',
    serviceHealthTitle: '플랫폼 지원', serviceHealthDescription: '기업에는 가용 상태만 표시합니다. 서비스 기능, 모델, 인프라는 HuanShu 플랫폼에서 관리합니다.', serviceHealthAction: '플랫폼에 문의',
    retry: '다시 불러오기', loadFailed: '기업 관리 정보를 일시적으로 불러올 수 없습니다',
    itemCopy: {
      license_or_capability_attention: { title: '서비스 기능 확인 필요', description: '플랫폼에서 기업 서비스 구성을 확인해야 합니다.', action: '서비스 상태 보기' },
      edge_node_offline: { title: '에지 노드 오프라인', description: '일부 기업 에지 노드가 플랫폼에 연결되어 있지 않습니다.', action: '서비스 상태 보기' },
      operating_analysis_access_gap: { title: '운영 분석 권한 미완료', description: '일부 활성 구성원에게 운영 분석 권한이 없습니다.', action: '구성원 관리' },
      pending_invitations: { title: '구성원 초대 대기', description: '발송한 초대가 아직 수락되지 않았습니다.', action: '구성원 관리' },
      knowledge_processing_failed: { title: '지식 처리 실패', description: '일부 지식 콘텐츠를 다시 처리해야 합니다.', action: '지식 베이스 보기' },
      recent_high_risk_operations: { title: '최근 중요 권한 변경', description: '최근 7일의 중요 작업을 검토해야 합니다.', action: '감사 기록 보기' },
    },
  },
}

export function getEnterpriseAdministrationCopy(locale: string): EnterpriseAdministrationCopy {
  return enterpriseAdministrationCopyByLocale[locale] || enterpriseAdministrationCopyByLocale['zh-CN']
}
