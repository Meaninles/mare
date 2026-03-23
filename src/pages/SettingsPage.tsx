import { MonitorCog, MoonStar, SunMedium } from "lucide-react";
import { useTheme, type ThemeMode } from "../components/ThemeProvider";

const themeOptions: Array<{
  value: ThemeMode;
  title: string;
  subtitle: string;
  description: string;
  icon: typeof SunMedium;
}> = [
  {
    value: "light",
    title: "浅色主题",
    subtitle: "Mare Day",
    description: "偏银灰与雾紫的浅色方案，保留专业感同时更适合白天长时间浏览。",
    icon: SunMedium
  },
  {
    value: "dark",
    title: "深色主题",
    subtitle: "Mare Night",
    description: "偏石墨、雾紫与海洋青绿的深色方案，更贴近参考图的沉静科技感。",
    icon: MoonStar
  }
];

export function SettingsPage() {
  const { theme, setTheme } = useTheme();

  return (
    <section className="page-stack">
      <article className="hero-card">
        <p className="eyebrow">Settings</p>
        <h3>集中管理客户端外观、连接策略与后续配置恢复能力。</h3>
        <p>
          这不是网页设置页，而是 Mare 桌面客户端的统一偏好入口。当前先接入浅色与深色两套主题，交互结构保持一致，只切换配色风格。
        </p>
      </article>

      <div className="page-grid settings-grid">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">Appearance</p>
              <h4>客户端配色</h4>
            </div>
            <span className="status-pill subtle">默认浅色</span>
          </div>

          <div className="theme-option-grid">
            {themeOptions.map((option) => {
              const Icon = option.icon;
              const active = theme === option.value;

              return (
                <button
                  key={option.value}
                  type="button"
                  className={`theme-option-card${active ? " active" : ""}`}
                  onClick={() => setTheme(option.value)}
                >
                  <div className="theme-option-head">
                    <div className="theme-option-icon">
                      <Icon size={18} strokeWidth={1.9} />
                    </div>
                    <span className={`status-pill ${active ? "success" : "subtle"}`}>
                      {active ? "当前使用" : "点击切换"}
                    </span>
                  </div>

                  <div className={`theme-preview ${option.value}`}>
                    <div className="theme-preview-top" />
                    <div className="theme-preview-main">
                      <div className="theme-preview-sidebar" />
                      <div className="theme-preview-content">
                        <span className="theme-preview-line long" />
                        <span className="theme-preview-line" />
                        <span className="theme-preview-line short" />
                      </div>
                    </div>
                  </div>

                  <div className="theme-option-copy">
                    <strong>{option.title}</strong>
                    <span>{option.subtitle}</span>
                    <p>{option.description}</p>
                  </div>
                </button>
              );
            })}
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">Client Notes</p>
              <h4>当前偏好状态</h4>
            </div>
          </div>

          <div className="detail-list">
            <div className="settings-note-card">
              <MonitorCog size={18} />
              <div>
                <strong>当前主题</strong>
                <p>{theme === "light" ? "浅色主题 Mare Day" : "深色主题 Mare Night"}</p>
              </div>
            </div>
            <div className="settings-note-card">
              <SunMedium size={18} />
              <div>
                <strong>切换规则</strong>
                <p>浅色与深色共享同一套布局、动线与操作习惯，只切换视觉系统。</p>
              </div>
            </div>
            <div className="settings-note-card">
              <MoonStar size={18} />
              <div>
                <strong>后续扩展</strong>
                <p>这里后面会继续接入窗口偏好、端点重绑定和配置导入导出能力。</p>
              </div>
            </div>
          </div>
        </article>
      </div>
    </section>
  );
}
