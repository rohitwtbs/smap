{-# LANGUAGE DataKinds #-}
{-# LANGUAGE TypeOperators #-}
{-# LANGUAGE OverloadedStrings #-}

module Smap.Api where

import Servant
import Smap.Types
import Smap.Db
import Smap.Query.Parser
import Smap.Query.AST
import Smap.Db.Query
import Control.Monad.IO.Class (liftIO)
import Data.Pool (withResource)
import Data.Text (Text)
import qualified Database.PostgreSQL.Simple as PG

-- | Define the sMAP API
type SmapApi = 
    "add" :> Capture "key" Text :> ReqBody '[JSON] SmapPayload :> Post '[JSON] Text
    :<|> "api" :> "query" :> ReqBody '[PlainText] Text :> Post '[JSON] [Text]

-- | API Implementation
smapServer :: DbPool -> Server SmapApi
smapServer pool = handleAdd pool :<|> handleQuery pool

handleAdd :: DbPool -> Text -> SmapPayload -> Handler Text
handleAdd pool key payload = do
    -- TODO: Validate key and get subscription ID
    let subId = 1 
    liftIO $ insertPayload pool subId payload
    return "OK"

handleQuery :: DbPool -> Text -> Handler [Text]
handleQuery pool queryStr = do
    case parseQuery queryStr of
        Left err -> throwError $ err400 { errBody = "Parse Error" }
        Right (Select selQuery) -> do
            let (sql, params) = translateQuery selQuery
            res <- liftIO $ withResource pool $ \conn -> do
                PG.query conn sql params :: IO [PG.Only Text]
            return $ map PG.fromOnly res
        Right _ -> throwError $ err400 { errBody = "Only SELECT supported for now" }

-- | Entry point for the API
smapApp :: DbPool -> Application
smapApp pool = serve (Proxy :: Proxy SmapApi) (smapServer pool)
