{-# LANGUAGE OverloadedStrings #-}

module Smap.Db.Query (translateQuery) where

import Data.Text (Text)
import qualified Data.Text as T
import qualified Data.Text.Encoding as TE
import Smap.Query.AST
import qualified Database.PostgreSQL.Simple.Types as PQ

translateQuery :: SelectQuery -> (PQ.Query, [Text])
translateQuery SelectQuery{..} = 
    let (fieldsSql, fieldsParams) = translateFields selectFields
        (whereSql, whereParams) = maybe ("TRUE", []) translateWhere selectWhere
        limitSql = maybe "" (const " LIMIT ?") selectLimit
        limitParam = maybe [] (\l -> [T.pack $ show l]) selectLimit
        
        -- Basic join if we need data
        baseQuery = if any isDataField selectFields
                    then "SELECT " <> fieldsSql <> " FROM data d JOIN stream s ON d.sid = s.id WHERE " <> whereSql <> limitSql
                    else "SELECT " <> fieldsSql <> " FROM stream s WHERE " <> whereSql <> limitSql
    in (PQ.Query (TE.encodeUtf8 baseQuery), fieldsParams ++ whereParams ++ limitParam)

isDataField :: Field -> Bool
isDataField (DataField _) = True
isDataField _ = False

translateFields :: [Field] -> (Text, [Text])
translateFields [AllFields] = ("s.uuid, s.metadata", [])
translateFields fields = 
    let results = map translateField fields
    in (T.intercalate ", " (map fst results), concatMap snd results)

translateField :: Field -> (Text, [Text])
translateField UuidField = ("s.uuid", [])
translateField PathField = ("s.metadata->'Path'", [])
translateField (MetadataField k) = ("s.metadata->?", [k])
translateField (DataField _) = ("d.time, d.value", [])
translateField AllFields = ("s.uuid, s.metadata", [])

translateWhere :: WhereClause -> (Text, [Text])
translateWhere (Has k) = ("s.metadata ? ?", [k])
translateWhere (Equals k v) = ("s.metadata->? = ?", [k, v])
translateWhere (Like k v) = ("s.metadata->? LIKE ?", [k, v])
translateWhere (And a b) = 
    let (asql, ap) = translateWhere a
        (bsql, bp) = translateWhere b
    in ("(" <> asql <> " AND " <> bsql <> ")", ap ++ bp)
translateWhere (Or a b) = 
    let (asql, ap) = translateWhere a
        (bsql, bp) = translateWhere b
    in ("(" <> asql <> " OR " <> bsql <> ")", ap ++ bp)
translateWhere (Not a) = 
    let (asql, ap) = translateWhere a
    in ("(NOT " <> asql <> ")", ap)
-- TODO: Handle more complex clauses and TimeExprs
translateWhere _ = ("TRUE", [])
