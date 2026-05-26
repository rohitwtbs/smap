{-# LANGUAGE OverloadedStrings #-}

module Smap.Db where

import Database.PostgreSQL.Simple
import Database.PostgreSQL.Simple.Types (PGArray(..))
import Data.Pool
import Data.Text (Text)
import qualified Data.Text as T
import qualified Data.Text.Encoding as TE
import Data.Time.Clock.POSIX (utcTimeToPOSIXSeconds)
import Smap.Types
import Data.Map (Map)
import qualified Data.Map as M
import Data.UUID (UUID)
import Control.Monad (forM_)

type DbPool = Pool Connection

initPool :: String -> IO DbPool
initPool connStr = 
    let createConn = connectPostgreSQL (TE.encodeUtf8 $ T.pack connStr)
        destroyConn = close
        unusedTimeout = 10 -- seconds
        maxResources = 10
    in newPool $ defaultPoolConfig createConn destroyConn unusedTimeout maxResources

-- | Insert sMAP payload into the database
insertPayload :: DbPool -> Int -> SmapPayload -> IO ()
insertPayload pool subId payload = withResource pool $ \conn -> do
    withTransaction conn $ do
        forM_ (M.toList payload) $ \(path, sd) -> do
            -- 1. Ensure stream exists and get its ID
            [Only sid] <- query conn "SELECT add_stream(?, ?)" (subId, sdUuid sd) :: IO [Only Int]
            
            -- 2. Update metadata
            let metadata = sdMetadata sd
            if not (M.null metadata)
                then execute conn "UPDATE stream SET metadata = metadata || hstore(?::text[], ?::text[]) WHERE id = ?" 
                        (PGArray (M.keys metadata), PGArray (M.elems metadata), sid)
                else return 0
                
            -- 3. Insert readings
            let readings = sdReadings sd
            if not (null readings)
                then executeMany conn "INSERT INTO data (sid, time, value) VALUES (?, ?, ?)" 
                        (map (\r -> (sid, round (utcTimeToPOSIXSeconds (readingTime r) * 1000) :: Integer, readingValue r)) readings)
                else return 0
    return ()
