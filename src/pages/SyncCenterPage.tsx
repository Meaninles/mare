export function SyncCenterPage() {
  return (
    <section className="page-grid">
      <article className="hero-card">
        <p className="eyebrow">Sync Center</p>
        <h3>在这里承接副本补齐、恢复任务、失败重试和后续冲突处理。</h3>
        <p>当前阶段先完成统一主界面，为后续差异分析、恢复任务和批量补齐提供稳定入口与清晰视觉层级。</p>
      </article>

      <article className="detail-card">
        <h4>下一步将接入</h4>
        <ul className="detail-list">
          <li>待恢复资产列表</li>
          <li>同步任务与失败重试</li>
          <li>副本差异与冲突候选展示</li>
        </ul>
      </article>
    </section>
  );
}
