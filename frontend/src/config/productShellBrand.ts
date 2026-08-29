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
  capabilityListLabel: string
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
  recentWork: string
  noRecentWork: string
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
    eyebrow: '面向零售经营的一体化工作空间',
    headline: '让员工助理与经营分析，回到每天的业务现场。',
    description: '在同一个企业空间中，支持员工协作与可信的经营分析。',
    capabilityListLabel: '产品能力',
    employeeAssistant: '员工助理',
    operatingAnalysis: '经营分析',
  },
  'en-US': {
    eyebrow: 'One workspace for retail operations',
    headline: 'Bring employee assistance and operating analysis into daily retail work.',
    description: 'A single enterprise space for employee collaboration and trusted operating analysis.',
    capabilityListLabel: 'Product capabilities',
    employeeAssistant: 'Employee Assistant',
    operatingAnalysis: 'Operating Analysis',
  },
  'ru-RU': {
    eyebrow: 'Единое рабочее пространство для розницы',
    headline: 'Помощник сотрудника и операционный анализ для ежедневной работы в рознице.',
    description: 'Единое пространство предприятия для совместной работы и достоверного операционного анализа.',
    capabilityListLabel: 'Возможности продукта',
    employeeAssistant: 'Помощник сотрудника',
    operatingAnalysis: 'Операционный анализ',
  },
  'ko-KR': {
    eyebrow: '리테일 운영을 위한 하나의 업무 공간',
    headline: '직원 지원과 운영 분석을 매일의 리테일 업무로 가져옵니다.',
    description: '직원 협업과 신뢰할 수 있는 운영 분석을 위한 하나의 엔터프라이즈 공간입니다.',
    capabilityListLabel: '제품 기능',
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
    headline: '让知识、分析与经营行动，形成一个持续改善的闭环。',
    compactHeadline: '知识驱动协作，数据辅助经营。',
    description: '从员工每天的业务问题出发，连接企业知识与受治理经营数据，逐步走向更可靠的经营决策。',
    currentWork: '当前工作',
    employeeDescription: '问制度、查流程、找知识，在统一企业空间中完成日常协作。',
    analysisDescription: '基于企业受治理数据看指标、找异常，形成可追溯的经营判断。',
    startWork: '开始工作',
    enterWork: '进入经营分析',
    continueWork: '继续最近工作',
    recentWork: '最近工作',
    noRecentWork: '还没有最近工作',
    contactAdmin: '请联系企业管理员开通经营分析权限',
    serviceUnavailable: '经营分析服务暂不可用',
    loopTitle: '从业务现场出发，回到业务现场',
    loopDescription: '环枢把零售智能体理解为持续运转的经营能力，而不是一组分散工具。',
    loopSteps: ['企业知识与业务数据', '员工执行与协作', '经营分析与判断', '决策改进与沉淀'],
    roadmapTitle: '能力沿经营闭环逐步展开',
    roadmapDescription: '当前先把员工助理和经营分析做好，后续再进入更接近业务执行的环节。',
    current: '当前可用',
    planned: '规划中',
    procurementOrdering: '采购与订货',
  },
  'en-US': {
    eyebrow: 'HuanShu Retail Agent',
    headline: 'Turn knowledge, analysis, and action into a continuous operating loop.',
    compactHeadline: 'Knowledge for work. Data for decisions.',
    description: 'Start with everyday employee questions, connect enterprise knowledge and governed data, and build toward more reliable decisions.',
    currentWork: 'Current work',
    employeeDescription: 'Find policies, processes, and knowledge in one enterprise workspace.',
    analysisDescription: 'Use governed enterprise data to inspect metrics, find anomalies, and form traceable judgments.',
    startWork: 'Start working',
    enterWork: 'Open operating analysis',
    continueWork: 'Continue recent work',
    recentWork: 'Recent work',
    noRecentWork: 'No recent work yet',
    contactAdmin: 'Contact an enterprise administrator for operating analysis access',
    serviceUnavailable: 'Operating analysis is temporarily unavailable',
    loopTitle: 'Start from operations, return to operations',
    loopDescription: 'HuanShu treats a retail agent as a continuous operating capability, not a collection of separate tools.',
    loopSteps: ['Knowledge and business data', 'Employee execution', 'Operating analysis', 'Decisions and learning'],
    roadmapTitle: 'Capabilities expand along the operating loop',
    roadmapDescription: 'We are starting with employee assistance and operating analysis, then moving closer to business execution.',
    current: 'Available now',
    planned: 'Planned',
    procurementOrdering: 'Procurement and ordering',
  },
  'ru-RU': {
    eyebrow: 'Розничный агент HuanShu',
    headline: 'Объедините знания, анализ и действия в непрерывный операционный цикл.',
    compactHeadline: 'Знания для работы. Данные для решений.',
    description: 'Начните с ежедневных вопросов сотрудников, свяжите знания и управляемые данные и переходите к более надежным решениям.',
    currentWork: 'Текущая работа',
    employeeDescription: 'Находите правила, процессы и знания в едином пространстве предприятия.',
    analysisDescription: 'Изучайте показатели и отклонения на основе управляемых данных предприятия.',
    startWork: 'Начать работу',
    enterWork: 'Открыть операционный анализ',
    continueWork: 'Продолжить последнюю работу',
    recentWork: 'Последняя работа',
    noRecentWork: 'Недавних задач пока нет',
    contactAdmin: 'Обратитесь к администратору предприятия для получения доступа',
    serviceUnavailable: 'Операционный анализ временно недоступен',
    loopTitle: 'От операций — обратно к операциям',
    loopDescription: 'HuanShu рассматривает розничного агента как непрерывную операционную способность, а не набор отдельных инструментов.',
    loopSteps: ['Знания и бизнес-данные', 'Работа сотрудников', 'Операционный анализ', 'Решения и обучение'],
    roadmapTitle: 'Возможности развиваются вдоль операционного цикла',
    roadmapDescription: 'Сначала — помощь сотрудникам и операционный анализ, затем — более близкие к исполнению процессы.',
    current: 'Доступно сейчас',
    planned: 'В планах',
    procurementOrdering: 'Закупки и заказы',
  },
  'ko-KR': {
    eyebrow: 'HuanShu 리테일 에이전트',
    headline: '지식, 분석, 실행을 지속적으로 개선되는 운영 순환으로 연결합니다.',
    compactHeadline: '지식으로 협업하고, 데이터로 판단합니다.',
    description: '직원의 일상 질문에서 시작해 기업 지식과 거버넌스 데이터를 연결하고 더 신뢰할 수 있는 의사결정으로 확장합니다.',
    currentWork: '현재 업무',
    employeeDescription: '하나의 기업 공간에서 규정, 절차, 지식을 찾고 일상 협업을 수행합니다.',
    analysisDescription: '거버넌스가 적용된 기업 데이터로 지표와 이상 징후를 확인하고 추적 가능한 판단을 만듭니다.',
    startWork: '업무 시작',
    enterWork: '운영 분석 열기',
    continueWork: '최근 업무 계속하기',
    recentWork: '최근 업무',
    noRecentWork: '최근 업무가 없습니다',
    contactAdmin: '운영 분석 권한은 기업 관리자에게 문의하세요',
    serviceUnavailable: '운영 분석 서비스를 일시적으로 사용할 수 없습니다',
    loopTitle: '업무 현장에서 시작해 다시 현장으로',
    loopDescription: 'HuanShu는 리테일 에이전트를 분리된 도구 모음이 아닌 지속적인 운영 역량으로 봅니다.',
    loopSteps: ['기업 지식과 업무 데이터', '직원 실행과 협업', '운영 분석과 판단', '의사결정 개선과 축적'],
    roadmapTitle: '운영 순환을 따라 역량을 확장합니다',
    roadmapDescription: '직원 지원과 운영 분석을 먼저 완성한 뒤 실제 업무 실행에 더 가까운 단계로 확장합니다.',
    current: '현재 사용 가능',
    planned: '계획 중',
    procurementOrdering: '구매 및 발주',
  },
}

export function getRetailAgentHomeCopy(locale: string): RetailAgentHomeCopy {
  return homeCopyByLocale[locale] || homeCopyByLocale['zh-CN']
}
