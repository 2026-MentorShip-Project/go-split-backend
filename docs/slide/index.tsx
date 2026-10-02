import type { DesignSystem, Page, SlideMeta, SlideTransition } from '@open-slide/core';
import { useSlidePageNumber } from '@open-slide/core';
import type { ReactNode } from 'react';
import livePreview from './assets/2b-live-preview.png';
import settleShot from './assets/3a-settle.png';

export const design: DesignSystem = {
  palette: { bg: '#0b1e33', text: '#eaf6fb', accent: '#00add8' },
  fonts: {
    display: '"JetBrains Mono", "SF Mono", Menlo, Consolas, "PingFang TC", "Noto Sans TC", "Microsoft JhengHei", monospace',
    body: '-apple-system, BlinkMacSystemFont, "PingFang TC", "Noto Sans TC", "Microsoft JhengHei", system-ui, sans-serif',
  },
  typeScale: { hero: 200, body: 40 },
  radius: 16,
};

const muted = '#7fa3b8';
const rule = 'rgba(127, 163, 184, 0.35)';
const surface = '#11294a';
const warn = '#ffb454';

const EASE_OUT = 'cubic-bezier(0, 0, 0.2, 1)';
const EASE_IN = 'cubic-bezier(0.4, 0, 1, 1)';
const HOLD: Keyframe[] = [{ opacity: 1 }, { opacity: 1 }];

export const transition: SlideTransition = {
  duration: 260,
  exit: { duration: 260, easing: EASE_IN, keyframes: HOLD },
  enter: {
    duration: 260,
    easing: EASE_OUT,
    keyframes: [
      { opacity: 0, transform: 'translateY(6px)' },
      { opacity: 1, transform: 'translateY(0)' },
    ],
  },
};

const settle: SlideTransition = {
  duration: 280,
  exit: { duration: 280, easing: EASE_IN, keyframes: HOLD },
  enter: {
    duration: 280,
    easing: EASE_OUT,
    keyframes: [
      { opacity: 0, transform: 'translateY(12px)', filter: 'blur(4px)' },
      { opacity: 1, transform: 'translateY(0)', filter: 'blur(0)' },
    ],
  },
};

const mono = 'var(--osd-font-display)';

const Footer = ({ section }: { section: string }) => {
  const { current, total } = useSlidePageNumber();
  return (
    <div
      style={{
        position: 'absolute',
        left: 140,
        right: 140,
        bottom: 52,
        display: 'flex',
        justifyContent: 'space-between',
        fontFamily: mono,
        fontSize: 22,
        letterSpacing: '0.14em',
        color: muted,
      }}
    >
      <span>GO-SPLIT · {section}</span>
      <span>
        {String(current).padStart(2, '0')} / {String(total).padStart(2, '0')}
      </span>
    </div>
  );
};

const Frame = ({
  section,
  children,
  center = false,
}: {
  section?: string;
  children: ReactNode;
  center?: boolean;
}) => (
  <div
    style={{
      position: 'relative',
      width: '100%',
      height: '100%',
      boxSizing: 'border-box',
      padding: '120px 140px',
      background: 'var(--osd-bg)',
      color: 'var(--osd-text)',
      fontFamily: 'var(--osd-font-body)',
      display: 'flex',
      flexDirection: 'column',
      justifyContent: center ? 'center' : 'flex-start',
    }}
  >
    {children}
    {section && <Footer section={section} />}
  </div>
);

const Eyebrow = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      fontFamily: mono,
      fontSize: 24,
      letterSpacing: '0.18em',
      color: 'var(--osd-accent)',
      marginBottom: 28,
    }}
  >
    {children}
  </div>
);

const Heading = ({ children, size = 72 }: { children: ReactNode; size?: number }) => (
  <h2
    style={{
      fontFamily: mono,
      fontSize: size,
      fontWeight: 800,
      lineHeight: 1.15,
      letterSpacing: '-0.02em',
      margin: 0,
    }}
  >
    {children}
  </h2>
);

const Phone = ({ src, alt }: { src: string; alt: string }) => (
  <img
    src={src}
    alt={alt}
    style={{
      height: 760,
      borderRadius: 36,
      border: `3px solid ${rule}`,
      boxShadow: '0 30px 80px rgba(0, 0, 0, 0.45)',
      flexShrink: 0,
    }}
  />
);

// ---------- 01 Cover ----------

const Credit = ({ role, name }: { role: string; name: string }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      gap: 16,
      padding: '12px 24px',
      borderRadius: 999,
      border: `2px solid ${rule}`,
      background: surface,
      fontSize: 26,
    }}
  >
    <span style={{ fontFamily: mono, color: 'var(--osd-accent)' }}>{role}</span>
    <span>{name}</span>
  </div>
);

const Cover: Page = () => (
  <Frame center>
    <Eyebrow>MENTORSHIP 2026 · 專案總覽</Eyebrow>
    <h1
      style={{
        fontFamily: mono,
        fontSize: 'var(--osd-size-hero)',
        fontWeight: 800,
        letterSpacing: '-0.04em',
        lineHeight: 1.05,
        margin: 0,
      }}
    >
      Go<span style={{ color: 'var(--osd-accent)' }}>-</span>Split
    </h1>
    <p style={{ fontSize: 48, lineHeight: 1.4, color: muted, margin: '40px 0 0' }}>
      用規則分帳，不必逐筆手算。
    </p>
    <div
      style={{
        marginTop: 96,
        paddingTop: 32,
        borderTop: `2px dashed ${rule}`,
        width: 1100,
        fontFamily: mono,
        fontSize: 36,
        color: 'var(--osd-accent)',
      }}
    >
      NT$ 500 → 200 · 200 · 100 · 0
    </div>
    <div style={{ display: 'flex', gap: 24, marginTop: 48 }}>
      <Credit role="前端" name="Fane（航海士）" />
      <Credit role="後端" name="Will（水手）" />
    </div>
  </Frame>
);
Cover.transition = settle;

// ---------- 02 Problem ----------

const Problem: Page = () => (
  <Frame section="產品展示" center>
    <Heading size={104}>
      吃素的人，
      <br />
      不該付<span style={{ color: 'var(--osd-accent)' }}>肉錢</span>。
    </Heading>
    <p style={{ fontSize: 'var(--osd-size-body)', lineHeight: 1.5, color: muted, margin: '56px 0 0' }}>
      團體分帳的難處不在算術，而在「誰分攤什麼」。
    </p>
  </Frame>
);

// ---------- 03 Rules (UX) ----------

const ShareRow = ({
  name,
  reason,
  amount,
  dim = false,
}: {
  name: string;
  reason: string;
  amount: string;
  dim?: boolean;
}) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'baseline',
      padding: '18px 0',
      borderBottom: `2px dashed ${rule}`,
      fontFamily: mono,
      color: dim ? muted : 'var(--osd-text)',
    }}
  >
    <span style={{ fontSize: 40, width: 200 }}>{name}</span>
    <span style={{ fontSize: 26, flex: 1, color: muted }}>{reason}</span>
    <span style={{ fontSize: 40, color: dim ? muted : 'var(--osd-accent)' }}>{amount}</span>
  </div>
);

const Rules: Page = () => (
  <Frame section="產品展示">
    <div style={{ display: 'flex', alignItems: 'center', gap: 140, flex: 1 }}>
      <div style={{ flex: 1 }}>
        <Eyebrow>由規則決定分攤</Eyebrow>
        <Heading>
          為明細加上標籤，
          <br />
          立刻看到分攤。
        </Heading>
        <div style={{ fontFamily: mono, fontSize: 28, color: muted, margin: '56px 0 8px' }}>
          NT$ 500 · #肉品
        </div>
        <ShareRow name="Will" reason="其他人員" amount="200" />
        <ShareRow name="Ben" reason="其他人員" amount="200" />
        <ShareRow name="Coco" reason="#小孩 · 權重 ×0.5" amount="100" />
        <ShareRow name="Amy" reason="#吃素 · 不計入" amount="0" dim />
      </div>
      <Phone src={livePreview} alt="新增開銷表單預覽每位成員的分攤" />
    </div>
  </Frame>
);

// ---------- 04 One engine ----------

const Node = ({
  label,
  title,
  note,
  hero = false,
}: {
  label: string;
  title: string;
  note: string;
  hero?: boolean;
}) => (
  <div
    style={{
      width: 440,
      padding: '40px 44px',
      borderRadius: 'var(--osd-radius)',
      background: hero ? 'var(--osd-accent)' : surface,
      color: hero ? 'var(--osd-bg)' : 'var(--osd-text)',
      border: hero ? 'none' : `2px solid ${rule}`,
      boxSizing: 'border-box',
    }}
  >
    <div style={{ fontFamily: mono, fontSize: 22, letterSpacing: '0.14em', opacity: 0.75 }}>{label}</div>
    <div style={{ fontFamily: mono, fontSize: 40, fontWeight: 800, margin: '16px 0' }}>{title}</div>
    <div style={{ fontSize: 28, lineHeight: 1.4, opacity: hero ? 0.85 : 1, color: hero ? undefined : muted }}>
      {note}
    </div>
  </div>
);

const Arrow = ({ dir }: { dir: '←' | '→' }) => (
  <div style={{ fontFamily: mono, fontSize: 64, color: 'var(--osd-accent)' }}>{dir}</div>
);

const OneEngine: Page = () => (
  <Frame section="01 · 應用程式架構">
    <Eyebrow>關鍵決策</Eyebrow>
    <Heading>同一個引擎，跑在兩處。</Heading>
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginTop: 120,
      }}
    >
      <Node label="瀏覽器 · WASM" title="即時預覽" note="輸入時即算出分攤，只有儲存才送到伺服器。" />
      <Arrow dir="←" />
      <Node hero label="GO 套件" title="splitengine" note="純計算，沒有 I/O。" />
      <Arrow dir="→" />
      <Node label="伺服器 · CLOUD RUN" title="正式結果" note="儲存時再驗證一次，以此為準。" />
    </div>
    <p style={{ fontSize: 'var(--osd-size-body)', color: muted, margin: '96px 0 0' }}>
      預覽與正式結果不會分歧。
    </p>
  </Frame>
);

// ---------- 05 Why this architecture ----------

const Hop = ({
  title,
  sub,
  hero = false,
  width = 340,
}: {
  title: string;
  sub: string;
  hero?: boolean;
  width?: number;
}) => (
  <div
    style={{
      width,
      padding: '36px 32px',
      borderRadius: 'var(--osd-radius)',
      background: surface,
      border: `2px solid ${hero ? 'var(--osd-accent)' : rule}`,
      boxSizing: 'border-box',
    }}
  >
    <div style={{ fontFamily: mono, fontSize: 36, fontWeight: 800 }}>{title}</div>
    <div style={{ fontSize: 26, lineHeight: 1.4, color: muted, marginTop: 12 }}>{sub}</div>
  </div>
);

const Chain = ({ children }: { children: ReactNode }) => (
  <div style={{ fontFamily: mono, fontSize: 24, color: 'var(--osd-accent)' }}>{children}</div>
);

const StackCol = ({ label, children }: { label: string; children: ReactNode }) => (
  <div style={{ flex: 1 }}>
    <div style={{ fontFamily: mono, fontSize: 22, letterSpacing: '0.14em', color: 'var(--osd-accent)' }}>
      {label}
    </div>
    <div style={{ fontSize: 26, lineHeight: 1.4, color: muted, marginTop: 8 }}>{children}</div>
  </div>
);

const RequestPath: Page = () => (
  <Frame section="01 · 應用程式架構">
    <Eyebrow>一個請求的完整路徑</Eyebrow>
    <Heading>從瀏覽器到 Postgres。</Heading>
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginTop: 72,
      }}
    >
      <Hop title="瀏覽器" sub="Next.js · React 19" />
      <Arrow dir="→" />
      <Hop title="Vercel" sub="/api/* 同源 rewrite" />
      <Arrow dir="→" />
      <Hop hero title="Cloud Run" sub="Gin API · 0–5 個 instance" />
      <Arrow dir="→" />
      <Hop title="Cloud SQL" sub="PostgreSQL 15" />
    </div>
    <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 32, paddingRight: 300 }}>
      <Chain>session → 活動鎖 → 角色 → handler</Chain>
    </div>
    <div
      style={{
        marginTop: 64,
        paddingTop: 24,
        borderTop: `2px dashed var(--osd-accent)`,
        fontFamily: mono,
        fontSize: 32,
        color: 'var(--osd-accent)',
      }}
    >
      ← 回應：交易 COMMIT 後，才經 Vercel 送回瀏覽器
    </div>
    <div style={{ display: 'flex', gap: 48, marginTop: 56 }}>
      <StackCol label="前端">
        Next.js 16 · React 19
        <br />
        TypeScript · Tailwind v4
        <br />
        Zustand
      </StackCol>
      <StackCol label="後端">
        Go 1.25 · Gin
        <br />
        pgx/v5 · logrus
        <br />
        Swagger（swaggo）
      </StackCol>
      <StackCol label="分攤引擎">
        Go → WASM
        <br />
        npm @go-split/engine
      </StackCol>
      <StackCol label="基礎設施">
        Cloud Run · Cloud SQL
        <br />
        Vercel · Terraform
        <br />
        GitHub Actions
      </StackCol>
    </div>
  </Frame>
);

const Reason = ({ because, choice }: { because: string; choice: string }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'baseline',
      padding: '18px 0',
      borderBottom: `2px dashed ${rule}`,
    }}
  >
    <span style={{ width: 760, fontSize: 30, color: muted }}>{because}</span>
    <span style={{ width: 80, fontFamily: mono, fontSize: 32, color: 'var(--osd-accent)' }}>→</span>
    <span style={{ flex: 1, fontSize: 36 }}>{choice}</span>
  </div>
);

const WhyArchitecture: Page = () => (
  <Frame section="01 · 應用程式架構">
    <Eyebrow>每個選擇都對應一個產品限制</Eyebrow>
    <Heading>為什麼是這個架構？</Heading>
    <div style={{ marginTop: 56 }}>
      <Reason because="預覽必須即時，且與結果一致" choice="同一個 Go 引擎編譯成 WASM" />
      <Reason because="沒有自有網域" choice="經 Vercel 代理，cookie 維持第一方" />
      <Reason because="結算後結果不能變" choice="凍結快照，讀取不再重算" />
      <Reason because="用量集中在活動期間，平時閒置" choice="Cloud Run 縮減至零" />
      <Reason because="小團隊、時程有限" choice="託管服務 + Terraform" />
    </div>
  </Frame>
);

// ---------- Delivery pipeline ----------

const Lane = ({ label, children }: { label: string; children: ReactNode }) => (
  <div style={{ display: 'flex', alignItems: 'center' }}>
    <div style={{ width: 220, fontFamily: mono, fontSize: 24, letterSpacing: '0.14em', color: muted }}>{label}</div>
    <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>{children}</div>
  </div>
);

const Pipeline: Page = () => (
  <Frame section="01 · 應用程式架構">
    <Eyebrow>建置與部署</Eyebrow>
    <Heading>每次合併都自動上線。</Heading>
    <div style={{ display: 'flex', flexDirection: 'column', gap: 48, marginTop: 80 }}>
      <Lane label="後端">
        <Hop width={300} title="PR" sub="到 main" />
        <Arrow dir="→" />
        <Hop width={300} title="CI" sub="race、lint、E2E、k6" />
        <Arrow dir="→" />
        <Hop width={300} title="merge" sub="push 到 main" />
        <Arrow dir="→" />
        <Hop width={300} hero title="CD" sub="Terraform → Cloud Run" />
      </Lane>
      <Lane label="前端">
        <Hop width={300} title="PR" sub="到 main" />
        <Arrow dir="→" />
        <Hop width={300} title="CI" sub="Vercel Preview：建置 + 預覽網址" />
        <Arrow dir="→" />
        <Hop width={300} title="merge" sub="push 到 main" />
        <Arrow dir="→" />
        <Hop width={300} hero title="Vercel" sub="合併後自動上線" />
      </Lane>
    </div>
    <p style={{ fontSize: 'var(--osd-size-body)', color: muted, margin: '64px 0 0' }}>
      引擎以 engine tag 發布到 npm，前端建置時複製 engine.wasm。
    </p>
  </Frame>
);

// ---------- 06 Consistency ----------

const Layer = ({ n, children }: { n: string; children: ReactNode }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'baseline',
      gap: 40,
      padding: '22px 0',
      borderBottom: `2px dashed ${rule}`,
      fontSize: 36,
    }}
  >
    <span style={{ fontFamily: mono, color: 'var(--osd-accent)', fontSize: 32 }}>{n}</span>
    <span>{children}</span>
  </div>
);

// ---------- 07 Settle via host ----------

const HubDiagram = () => (
  <svg width={1060} height={640} viewBox="0 0 1060 640" style={{ fontFamily: 'var(--osd-font-display)' }}>
    <defs>
      <marker id="gs-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto">
        <path d="M0,0 L10,5 L0,10 z" fill="#00add8" />
      </marker>
    </defs>
    <line x1="190" y1="170" x2="425" y2="262" stroke="#00add8" strokeWidth="4" markerEnd="url(#gs-arrow)" />
    <line x1="190" y1="470" x2="425" y2="378" stroke="#00add8" strokeWidth="4" markerEnd="url(#gs-arrow)" />
    <line x1="650" y1="320" x2="880" y2="320" stroke="#00add8" strokeWidth="4" markerEnd="url(#gs-arrow)" />
    <text x="290" y="182" fill="#eaf6fb" fontSize="38" textAnchor="middle">840</text>
    <text x="290" y="490" fill="#eaf6fb" fontSize="38" textAnchor="middle">242</text>
    <text x="765" y="296" fill="#eaf6fb" fontSize="38" textAnchor="middle">332</text>

    <circle cx="120" cy="130" r="72" fill={surface} stroke={rule} strokeWidth="3" />
    <text x="120" y="143" fill="#eaf6fb" fontSize="36" textAnchor="middle">Ben</text>
    <circle cx="120" cy="510" r="72" fill={surface} stroke={rule} strokeWidth="3" />
    <text x="120" y="523" fill="#eaf6fb" fontSize="36" textAnchor="middle">Coco</text>
    <circle cx="960" cy="320" r="72" fill={surface} stroke={rule} strokeWidth="3" />
    <text x="960" y="333" fill="#eaf6fb" fontSize="36" textAnchor="middle">Amy</text>

    <circle cx="540" cy="320" r="110" fill="#00add8" />
    <text x="540" y="312" fill="#0b1e33" fontSize="44" fontWeight="800" textAnchor="middle">Will</text>
    <text x="540" y="352" fill="#0b1e33" fontSize="24" textAnchor="middle" letterSpacing="3">主辦人</text>
    <text x="540" y="492" fill="#00add8" fontSize="40" textAnchor="middle">應收 +750</text>
  </svg>
);

const SettleHub: Page = () => (
  <Frame section="產品展示">
    <div style={{ display: 'flex', alignItems: 'center', gap: 80, flex: 1 }}>
      <div style={{ flex: 1 }}>
        <Eyebrow>結算</Eyebrow>
        <Heading>所有轉帳都經過主辦人。</Heading>
        <div style={{ marginTop: 24 }}>
          <HubDiagram />
        </div>
      </div>
      <Phone src={settleShot} alt="結算預覽：經由主辦人的轉帳" />
    </div>
  </Frame>
);

// ---------- 08 Scale ----------

const ScaleDesign: Page = () => (
  <Frame section="03 · 擴展性">
    <Eyebrow>設計上為擴展所做的事</Eyebrow>
    <Heading>為擴展做了什麼。</Heading>
    <div style={{ marginTop: 72 }}>
      <Item n="01" title="無狀態 API，水平擴展" detail="session 存在 PostgreSQL，0–5 個 instance" />
      <Item n="02" title="預覽在瀏覽器計算" detail="WASM 引擎，只有儲存才送到伺服器" />
      <Item n="03" title="靜態資源走 Vercel CDN" detail="頁面與 engine.wasm；/api 不快取" />
      <Item n="04" title="PGO 流程" detail="正式環境 CPU profile 存到 GCS，供 go build -pgo" />
    </div>
  </Frame>
);

const Cell = ({ value, sub, accent = false }: { value: string; sub: string; accent?: boolean }) => (
  <div style={{ width: 560 }}>
    <div
      style={{
        fontFamily: mono,
        fontSize: 52,
        fontWeight: 800,
        lineHeight: 1.2,
        color: accent ? 'var(--osd-accent)' : 'var(--osd-text)',
      }}
    >
      {value}
    </div>
    <div style={{ fontSize: 24, lineHeight: 1.4, color: muted, marginTop: 8 }}>{sub}</div>
  </div>
);

const CompareRow = ({ label, children }: { label: string; children: ReactNode }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'flex-start',
      padding: '20px 0',
      borderBottom: `2px dashed ${rule}`,
    }}
  >
    <div style={{ width: 420, fontFamily: mono, fontSize: 26, letterSpacing: '0.1em', color: muted, paddingTop: 16 }}>
      {label}
    </div>
    {children}
  </div>
);

const ColHead = ({ title, sub, accent = false }: { title: string; sub: string; accent?: boolean }) => (
  <div style={{ width: 560 }}>
    <div
      style={{
        fontFamily: mono,
        fontSize: 26,
        letterSpacing: '0.14em',
        color: accent ? 'var(--osd-accent)' : 'var(--osd-text)',
      }}
    >
      {title}
    </div>
    <div style={{ fontSize: 22, color: muted, marginTop: 6 }}>{sub}</div>
  </div>
);

const Scale: Page = () => (
  <Frame section="03 · 擴展性">
    <Eyebrow>K6 負載測試</Eyebrow>
    <Heading>本機 vs 正式環境</Heading>
    <div style={{ marginTop: 56 }}>
      <div style={{ display: 'flex', paddingBottom: 20, borderBottom: `2px solid ${rule}` }}>
        <div style={{ width: 420 }} />
        <ColHead title="本機" sub="Apple M2 筆電 · 2026-09-28" />
        <ColHead accent title="正式環境" sub="Cloud Run asia-east1 · 2026-10-01" />
      </div>
      <CompareRow label="吞吐量">
        <Cell value="300 RPS" sub="讀寫混合 · 0 錯誤" />
        <Cell accent value="~75 RPS" sub="僅讀取 · 0 錯誤" />
      </CompareRow>
      <CompareRow label="P95">
        <Cell value="385 ms" sub="300 RPS 時" />
        <Cell accent value="53 ms" sub="75 RPS 時 · 100 RPS 時約 7 s" />
      </CompareRow>
      <CompareRow label="瓶頸">
        <Cell value="連線池" sub="請求排隊等待 8 條連線" />
        <Cell accent value="共用資源" sub="5 × 1 vCPU，或約 20 條 DB 連線" />
      </CompareRow>
    </div>
  </Frame>
);

// ---------- 09 Security ----------

const Count = ({ n, label, color }: { n: string; label: string; color: string }) => (
  <div>
    <div style={{ fontFamily: mono, fontSize: 180, fontWeight: 800, lineHeight: 1, color }}>{n}</div>
    <div style={{ fontFamily: mono, fontSize: 28, letterSpacing: '0.14em', color: muted, marginTop: 16 }}>
      {label}
    </div>
  </div>
);

const Gap = ({ code, children }: { code: string; children: ReactNode }) => (
  <div style={{ display: 'flex', gap: 28 }}>
    <span style={{ fontFamily: mono, color: warn }}>{code}</span>
    <span>{children}</span>
  </div>
);

const Security: Page = () => (
  <Frame section="05 · 資安">
    <Eyebrow>OWASP TOP 10（2021）</Eyebrow>
    <Heading>3 項已處理，6 項缺口。</Heading>
    <div style={{ display: 'flex', gap: 120, marginTop: 96, alignItems: 'flex-start' }}>
      <Count n="3" label="已處理" color="var(--osd-accent)" />
      <Count n="6" label="缺口" color={warn} />
      <div style={{ flex: 1, fontSize: 32, lineHeight: 1.5 }}>
        <Gap code="A01">訪客可被冒用</Gap>
        <Gap code="A05">沒有 CSP 與 frame-ancestors</Gap>
        <Gap code="A06">沒有相依套件掃描</Gap>
        <Gap code="A07">/auth 沒有 rate limiting</Gap>
        <Gap code="A08">Actions 以 tag 固定，而非 SHA</Gap>
        <Gap code="A09">500 沒有記錄原因</Gap>
      </div>
    </div>
  </Frame>
);

// ---------- 10 Next ----------

const Item = ({ n, title, detail }: { n: string; title: string; detail: string }) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'baseline',
      gap: 40,
      padding: '20px 0',
      borderBottom: `2px dashed ${rule}`,
    }}
  >
    <span style={{ fontFamily: mono, color: 'var(--osd-accent)', fontSize: 30 }}>{n}</span>
    <span style={{ width: 800, fontSize: 36 }}>{title}</span>
    <span style={{ flex: 1, fontSize: 26, color: muted }}>{detail}</span>
  </div>
);

const Next: Page = () => (
  <Frame section="06 · 技術債">
    <Eyebrow>接下來</Eyebrow>
    <Heading>償還技術債。</Heading>
    <div style={{ marginTop: 72 }}>
      <Item n="01" title="資料庫連線接近上限（20／約 25）" detail="換更大的 Cloud SQL 方案或加連線池代理" />
      <Item n="02" title="資料庫單一 zone，沒有 standby" detail="需要可用性時改為 REGIONAL" />
      <Item n="03" title="部署不等 CI 就上線" detail="部署依賴 CI，terraform plan 需核准" />
      <Item n="04" title="看得到請求失敗，看不到原因" detail="JSON log、trace id、依 endpoint 的指標" />
      <Item n="05" title="前端沒有測試與錯誤頁面" detail="共用元件單元測試、E2E smoke test、error.tsx" />
    </div>
  </Frame>
);

// ---------- Part dividers ----------

const Divider = ({ part, title, children }: { part: string; title: string; children: ReactNode }) => (
  <Frame center>
    <Eyebrow>{part}</Eyebrow>
    <Heading size={120}>{title}</Heading>
    <div style={{ marginTop: 72, width: 1300 }}>{children}</div>
  </Frame>
);

const DemoDivider: Page = () => (
  <Divider part="第一部分 · 產品展示" title="產品展示">
    <Layer n="01">以邀請碼加入，不需帳號</Layer>
    <Layer n="02">為明細加上標籤，立刻看到分攤</Layer>
    <Layer n="03">結算：每人一筆轉帳，經過主辦人</Layer>
  </Divider>
);

const TechDivider: Page = () => (
  <Divider part="第二部分 · 技術簡報" title="技術設計">
    <Layer n="01">應用程式架構</Layer>
    <Layer n="02">擴展性</Layer>
    <Layer n="03">資安</Layer>
    <Layer n="04">技術債</Layer>
  </Divider>
);

// ---------- 11 Closing ----------

const Closing: Page = () => (
  <Frame center>
    <h1
      style={{
        fontFamily: mono,
        fontSize: 'var(--osd-size-hero)',
        fontWeight: 800,
        letterSpacing: '-0.04em',
        lineHeight: 1.05,
        margin: 0,
      }}
    >
      謝謝<span style={{ color: 'var(--osd-accent)' }}>。</span>
    </h1>
    <div
      style={{
        marginTop: 64,
        paddingTop: 32,
        borderTop: `2px dashed ${rule}`,
        width: 1100,
        fontFamily: mono,
        fontSize: 40,
        color: 'var(--osd-accent)',
      }}
    >
      go-split.vercel.app
    </div>
    <p style={{ fontSize: 'var(--osd-size-body)', color: muted, margin: '24px 0 0' }}>歡迎提問。</p>
    <div style={{ fontFamily: mono, fontSize: 22, letterSpacing: '0.14em', color: muted, marginTop: 96 }}>
      製作工具
    </div>
    <div style={{ display: 'flex', gap: 24, marginTop: 20 }}>
      <Credit role="簡報" name="open-slide" />
      <Credit role="影片" name="onetake" />
      <Credit role="全程" name="Claude / Codex" />
    </div>
  </Frame>
);
Closing.transition = settle;

export const meta: SlideMeta = {
  title: 'Go-Split：專案總覽',
  createdAt: '2026-10-02T14:29:07.167Z',
};

export default [
  Cover,
  Problem,
  DemoDivider,
  Rules,
  SettleHub,
  TechDivider,
  RequestPath,
  OneEngine,
  WhyArchitecture,
  Pipeline,
  ScaleDesign,
  Scale,
  Security,
  Next,
  Closing,
] satisfies Page[];

export const notes: (string | undefined)[] = [
  `【第一部分 · 產品展示，0:00–2:00】
Go-Split 用來分攤一場活動的團體開銷，例如旅行或烤肉。主辦人負責記帳，其他人以訪客身分加入。約 10 秒。`,
  `團體分帳難的不是算術，而是「誰分攤什麼」：吃素的人不付肉錢，小孩付一半。逐筆手動分攤最容易出錯。約 15 秒。`,
  `播放展示影片，約 90 秒，三個重點：
1. 訪客用邀請連結加入：email、手機、邀請碼，自己勾選 #吃素，不需帳號。
2. 主辦人新增 NT$500 的肉品並加上 #肉品 標籤，指出即時預覽：Coco 半價、Amy 不計入。
3. 結算：每位成員只有一筆轉帳，付給主辦人或由主辦人付給你。
影片無法播放時，接下來兩頁是第 2、3 點的截圖備案。`,
  `備案：第 2 點。NT$500 的肉品，Will 與 Ben 各 200，Coco 權重 ×0.5 付 100，Amy 因 #吃素 不計入。表單會隨輸入顯示每個人的金額與原因。`,
  `備案：第 3 點。Will 付給 Amy 332，向 Ben 收 840、向 Coco 收 242，應收 750。比成員之間互相轉帳的最少轉帳圖更容易理解。結算會凍結成快照。`,
  `【第二部分 · 技術簡報，2:00–7:00】
接下來五分鐘，依照報告的章節順序。約 10 秒。`,
  `先看一個請求怎麼走：前端呼叫同源的 /api，Vercel 轉送到 Cloud Run，Gin 依序檢查 session、鎖住活動、檢查角色，再由 handler 透過 Unix socket 存取 Cloud SQL。回應會先暫存，等交易 commit 後才經 Vercel 送回，失敗就 rollback，所以用戶端不會對沒存下來的資料收到成功。約 40 秒。`,
  `剛才影片裡的即時預覽在瀏覽器執行，但它跟伺服器是同一份 Go 程式碼，編譯成 WASM。主辦人輸入時不需要打 API，只有按下儲存才送到伺服器，這也減輕了伺服器負載。儲存時伺服器會再驗證一次，以伺服器的結果為準。所以預覽和正式結果不會分歧。約 45 秒。`,
  `每個選擇都對應一個產品限制。預覽必須即時且與結果一致，所以同一個 Go 引擎跑在瀏覽器。沒有自有網域，所以經 Vercel 代理讓 cookie 維持第一方，代價是每個請求多約 41 ms。結算後不能變，所以凍結快照，之後的讀取也不必重算。用量集中在活動期間，所以 Cloud Run 縮減至零。小團隊、時程有限，所以全用託管服務加 Terraform。約 50 秒。`,
  `部署流程分前後端。後端每個 PR 到 main 會跑 race 測試、lint、API E2E 與 k6 smoke test；合併 push 到 main 後，CD 跑 Terraform、migration，再部署到 Cloud Run。前端的 CI 就是 Vercel Preview：每個 PR 都會建置一次並產生預覽網址，建置失敗就看得出來，也能先在預覽網址上確認；合併到 main 就自動建置並上線。引擎另外以 engine tag 發布到 npm，前端建置時從套件複製 engine.wasm，升級版本就能換新引擎。約 40 秒。`,
  `擴展性先看設計上做了什麼。API 是無狀態的，session 存在 PostgreSQL，Cloud Run 可在 0 到 5 個 instance 間擴展。預覽在瀏覽器用 WASM 計算，主辦人輸入時不打 API，只有按下儲存才送到伺服器。頁面和 engine.wasm 從 Vercel CDN 載入，/api 則原封不動轉送、不快取。正式環境的 CPU profile 會存到 GCS，可用於 PGO。約 30 秒。`,
  `兩次 k6 測試，同一份程式碼。本機 M2 筆電上，讀寫混合 300 RPS，P95 385 ms、沒有錯誤；延遲主要是在等 8 條連線池的連線，不是 CPU。正式環境的 Cloud Run 只測讀取：75 RPS 以內 P95 約 53 ms，到 100 RPS 時請求開始排隊，P95 升到約 7 秒，但仍沒有錯誤。所有 endpoint 一起變慢，代表瓶頸是共用資源：5 個 1 vCPU 的 instance，或約 20 條資料庫連線。正式環境的 CPU 和資料庫都比筆電小得多。約 75 RPS 仍遠高於預期規模：單一活動很少每秒超過幾次儲存。約 50 秒。`,
  `對照 OWASP Top 10：加密、Injection、安全設計已處理，SSRF 不適用。還有六項缺口，最嚴重的是訪客冒用：訪客只以 email 和手機識別。另外 /auth 沒有限流、沒有 CSP、沒有相依套件掃描、Actions 以 tag 固定、500 沒有記錄原因。約 40 秒。`,
  `最後是刻意接受的技術債。5 個 instance 各 4 條連線共 20 條，接近約 25 條的上限，需要更大的方案或連線池代理。資料庫單一 zone 是為了省成本。部署還不會等 CI。可觀測性只靠 GCP 預設功能：看得到請求失敗、何時、哪個 instance，但多半看不出原因，也看不出哪個 endpoint 變慢，所以要改成 JSON log 加 trace id，並有依 endpoint 的指標。前端則需要測試和錯誤頁面。約 45 秒。`,
  `謝謝大家，歡迎提問。`,
];
