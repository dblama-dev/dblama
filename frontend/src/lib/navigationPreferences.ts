export interface TableLocation { scope:string;db:string;table:string;schema:string|null }
export interface NavigationPreferences { collapsed:boolean;width:number;recent:TableLocation[];favorites:TableLocation[] }
const key='dblama:navigation:v1'
const defaults=():NavigationPreferences=>({collapsed:false,width:220,recent:[],favorites:[]})
function browserStorage():Storage|undefined {try{return globalThis.localStorage}catch{return undefined}}
const validLocation=(v:unknown):v is TableLocation=>!!v && typeof v==='object' && typeof (v as TableLocation).scope==='string' && typeof (v as TableLocation).db==='string' && typeof (v as TableLocation).table==='string' && ((v as TableLocation).schema===null || typeof (v as TableLocation).schema==='string')
export function readNavigationPreferences(storage:Pick<Storage,'getItem'>|undefined=browserStorage()):NavigationPreferences{
 try{
  const v=JSON.parse(storage?.getItem(key)??'null')
  if(!v || typeof v!=='object')return defaults()
  return {collapsed:v.collapsed===true,width:typeof v.width==='number'&&Number.isFinite(v.width)?Math.min(420,Math.max(180,v.width)):220,recent:Array.isArray(v.recent)?v.recent.filter(validLocation).slice(0,12):[],favorites:Array.isArray(v.favorites)?v.favorites.filter(validLocation).slice(0,50):[]}
 }catch{return defaults()}
}
export function writeNavigationPreferences(storage:Pick<Storage,'setItem'>|undefined=browserStorage(),value:NavigationPreferences):void{try{storage?.setItem(key,JSON.stringify(value))}catch{/* Preferences are optional. */}}
const same=(a:TableLocation,b:TableLocation)=>a.scope===b.scope&&a.db===b.db&&a.table===b.table&&a.schema===b.schema
export function rememberTable(state:NavigationPreferences,table:TableLocation):NavigationPreferences{return {...state,recent:[table,...state.recent.filter(v=>!same(v,table))].slice(0,12)}}
export function toggleFavorite(state:NavigationPreferences,table:TableLocation):NavigationPreferences{return {...state,favorites:state.favorites.some(v=>same(v,table))?state.favorites.filter(v=>!same(v,table)):[table,...state.favorites].slice(0,50)}}
