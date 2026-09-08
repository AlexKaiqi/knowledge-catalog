"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const sessionKey = "kc.repository.login.v1:" + location.origin;
  let session = null, discovery = null, requestVersion = 0, loginVersion = 0;
  const repository = decodeURIComponent(location.pathname.slice("/repositories/".length));
  const showError = (error) => { $("error").textContent = error.message || String(error); $("error").hidden = false; };
  const clearError = () => { $("error").hidden = true; $("error").textContent = ""; };
  async function json(path, body, headers = {}) {
    const response = await fetch(path, {method: body === undefined ? "GET" : "POST", headers: {"Accept":"application/json", ...(body === undefined ? {} : {"Content-Type":"application/json"}), ...headers}, body: body === undefined ? undefined : JSON.stringify(body), credentials:"same-origin", redirect:"error", cache:"no-store"});
    const value = await response.json();
    if (!response.ok) { const error = new Error(value.error?.message || "请求失败，请稍后重试。"); error.code = value.error?.code; throw error; }
    return value;
  }
  async function authHeaders() {
    if (!session) return {};
    if (session.expiresAt && session.expiresAt <= Date.now()+30000) {
	  const prior=session, login=loginVersion;
      if (!session.refreshToken) throw new Error("登录已过期，请重新登录。");
      const renewed = await json("/identity/v1/token", {grantType:"refresh_token",refreshToken:session.refreshToken});
      const who = await json("/identity/v1/whoami", undefined, {Authorization:"Bearer "+renewed.accessToken});
	  if (prior!==session || login!==loginVersion) throw new Error("登录状态已改变，请重新操作。");
      if (who.principal !== session.principal) throw new Error("续期后的身份发生变化，请重新登录。");
      session = {...session,...renewed,expiresAt:Date.now()+renewed.expiresIn*1000};
      sessionStorage.setItem(sessionKey,JSON.stringify(session));
    }
    return session.local ? {"X-Kc-As":session.principal} : {Authorization:"Bearer "+session.accessToken};
  }
  async function api(path, body) { return json(path,body,await authHeaders()); }
  function clearRepository() {
    $("details").hidden=true;$("repo-name").textContent="仓库管理";document.title="仓库管理 · Knowledge Catalog";
    for (const id of ["owner","store","repo-id","catalog","repo-state","head","management-url","pack-command","head-command","live-status"]) $(id).textContent="";
    $("management-url").removeAttribute("href");
    $("projection").hidden=true;$("projection").textContent="";
  }
  function clearSession() {loginVersion++;session=null;sessionStorage.removeItem(sessionKey);clearRepository();$("authorization").hidden=true;$("browser-login").disabled=false;$("logout").hidden=true;$("identity").textContent="尚未登录";$("login").hidden=false;requestVersion++;}
  function safeLink(element, value) {
    const target = new URL(value,location.origin);
    if (!/^https?:$/.test(target.protocol) || target.username || target.password) throw new Error("服务返回的管理地址无效。");
    element.href=target.href;element.textContent=target.href;
  }
  async function load() {
    const version=++requestVersion;clearError();$("live-status").textContent="正在刷新…";
    try {
    const who=await api("/identity/v1/whoami");
    if (version!==requestVersion) return;
    $("identity").textContent=who.principal;$("logout").hidden=false;$("login").hidden=true;
    const item=await api("/catalog/v1/repositories/"+encodeURIComponent(repository));
    if (version!==requestVersion) return;
    $("login").hidden=true;$("details").hidden=false;$("logout").hidden=false;$("identity").textContent=who.principal;
    $("repo-name").textContent=item.name || "仓库管理";document.title=(item.name || "仓库管理")+" · Knowledge Catalog";
    for (const [id,value] of Object.entries({owner:item.owner,store:item.store,"repo-id":item.repositoryId,catalog:item.catalog,"repo-state":item.provisioningState === "READY" ? "可使用" : "正在准备",head:item.head || "尚无版本"})) $(id).textContent=value || "—";
    safeLink($("management-url"),item.managementURL);
    const readiness=item.readiness;
    $("projection").hidden=!readiness;
    if (readiness) {
      $("head").textContent=readiness.publishedCommit || "当前版本暂无法查询";
      const states={READY:"当前发布版本已可检索。",BUILDING:"当前发布版本的检索索引构建中。",UPDATING:"当前发布版本的检索索引更新中。",FAILED:"检索索引构建失败，请联系部署维护者处理；已发布的知识版本仍可读取。",RETIRED:"检索索引已停用；已发布的知识版本仍可读取。",NOT_READY:"当前发布版本尚无可用的检索索引。",UNAVAILABLE:"检索状态暂时无法确认，可继续查询发布版本。",NOT_CONFIGURED:"此部署尚未配置检索服务。",NOT_AUTHORIZED:"当前账号没有查询检索准备状态的权限。"};
      const guide=[];
      if (readiness.publication!=="PUBLISHED") guide.push("当前存储暂不可用；已保存的仓库地址仍保留。");
      if (readiness.profile==="MISSING") guide.push("请先发布源说明，帮助使用者理解这个知识源。");
      if (readiness.schemaCount===0) guide.push("尚未发布知识 Schema；请定义知识类型和查询字段。");
      guide.push(states[readiness.search] || "检索状态尚未就绪。");
      $("projection").textContent=guide.join(" ");
    }
    const quoted="'"+item.repositoryId.replaceAll("'","'\\''")+"'";
    $("pack-command").textContent="kc pack --repo "+quoted+" --dir <草稿目录> --out changeset.json";
    $("head-command").textContent="kc writer head --repo "+quoted;
    if (readiness) {$("live-status").textContent=readiness.publication==="PUBLISHED" ? "状态已更新" : "仓地址已恢复，当前存储暂不可用";return;}
    try {
      const head=await api("/writer/v1/repositories/"+encodeURIComponent(repository)+"/head");
      if (version===requestVersion) $("head").textContent=head.head || head.commit || JSON.stringify(head);
    } catch (error) {if (version===requestVersion) $("live-status").textContent="仓信息已恢复；当前发布版本暂无法查询："+error.message;return;}
    if (version===requestVersion) $("live-status").textContent="状态已更新";
    } catch(error) {if (version!==requestVersion) return;clearRepository();throw error;}
  }
  async function acceptLogin(candidate,version) {
    const headers=candidate.local ? {"X-Kc-As":candidate.principal} : {Authorization:"Bearer "+candidate.accessToken};
    const who=await json("/identity/v1/whoami",undefined,headers);
    if (version!==loginVersion) return;
    const verified={...candidate,principal:who.principal};
    sessionStorage.setItem(sessionKey,JSON.stringify(verified));session=verified;clearRepository();
    await load();
  }
  $("credentials").addEventListener("submit",async(event)=>{event.preventDefault();clearError();const version=++loginVersion;try {const value=$("credential").value;await acceptLogin(discovery.localAssertion ? {local:true,principal:value} : {accessToken:value.replace(/^Bearer\s+/i,"")},version);if(version===loginVersion)$("credential").value="";} catch(error){if(version===loginVersion)showError(error);}});
  $("logout").addEventListener("click",()=>{clearSession();clearError();});
  $("refresh").addEventListener("click",()=>load().catch(showError));
  const random = () => {const bytes=crypto.getRandomValues(new Uint8Array(32));return btoa(String.fromCharCode(...bytes)).replaceAll("+","-").replaceAll("/","_").replaceAll("=","");};
  $("browser-login").addEventListener("click",async()=>{
    clearError();$("browser-login").disabled=true;
    const version=++loginVersion;
    try {
      const verifier=random();const state=random();
      const digest=new Uint8Array(await crypto.subtle.digest("SHA-256",new TextEncoder().encode(verifier)));
      const challenge=btoa(String.fromCharCode(...digest)).replaceAll("+","-").replaceAll("/","_").replaceAll("=","");
      const pending=await json("/identity/v1/authorize",{codeChallenge:challenge,state});
      if (version!==loginVersion) return;
      safeLink($("authorization-link"),pending.authorizationURL);$("authorization-link").textContent="打开登录页面";$("authorization").hidden=false;
      const until=Date.now()+Math.min(pending.expiresIn,300)*1000;
      while (Date.now()<until && version===loginVersion) {
        await new Promise(resolve=>setTimeout(resolve,3000));
        if (version!==loginVersion) return;
        const poll=await json("/identity/v1/authorize:poll",{requestURI:pending.requestURI});
        if (version!==loginVersion) return;
        if (poll.status!=="completed") continue;
        const tokens=await json("/identity/v1/token",{grantType:"authorization_code",code:poll.code,codeVerifier:verifier,redirectURI:poll.redirectURI});
        if (version!==loginVersion) return;
        await acceptLogin({...tokens,expiresAt:Date.now()+tokens.expiresIn*1000},version);if(version===loginVersion)$("authorization").hidden=true;return;
      }
      throw new Error("本次登录已过期，请重新发起。");
    } catch(error){if(version===loginVersion)showError(error);} finally {if(version===loginVersion)$("browser-login").disabled=false;}
  });
  async function start() {
    discovery=await json("/identity/v1/auth");
    try {session=JSON.parse(sessionStorage.getItem(sessionKey)||"null");}catch{sessionStorage.removeItem(sessionKey);}
    if (discovery.localAssertion) {$("credentials").hidden=false;$("login-description").textContent="这是部署明确启用的本地测试登录。请输入 KC 用户名。";}
    else if (discovery.browserLogin) {$("browser-login").hidden=false;}
    else {$("credentials").hidden=false;$("credential-label").textContent="访问凭证";$("credential").type="password";$("credential").autocomplete="off";$("login-description").textContent="此部署使用访问凭证登录，也会自动尝试已有的组织网页登录。";}
    try {await load();}catch(error){if (error.code==="UNAUTHENTICATED" || !session) {clearSession();} else {showError(error);$("logout").hidden=false;$("identity").textContent=session.principal || "已登录";}}
  }
  start().catch(showError);
})();
