{-# LANGUAGE OverloadedStrings #-}
{-# LANGUAGE DeriveGeneric #-}
{-# LANGUAGE RecordWildCards #-}

module Smap.Types
    ( Reading(..)
    , StreamData(..)
    , SmapPayload
    ) where

import GHC.Generics
import Data.Text (Text)
import qualified Data.Text as T
import Data.Map (Map)
import qualified Data.Map as M
import Data.UUID (UUID)
import Data.Time (UTCTime)
import Data.Time.Clock.POSIX (posixSecondsToUTCTime, utcTimeToPOSIXSeconds)
import Data.Aeson
import Data.Aeson.Types (Parser)
import qualified Data.Aeson.KeyMap as KM
import qualified Data.Aeson.Key as K
import qualified Data.Vector as V

-- | A single sMAP reading: (timestamp, value)
data Reading = Reading
    { readingTime  :: UTCTime
    , readingValue :: Double
    } deriving (Show, Eq, Generic)

-- sMAP readings are represented as [timestamp, value] in JSON
instance FromJSON Reading where
    parseJSON = withArray "Reading" $ \arr -> do
        if V.length arr /= 2
            then fail "Reading must be an array of [timestamp, value]"
            else do
                ts <- parseJSON (arr V.! 0) :: Parser Double
                val <- parseJSON (arr V.! 1) :: Parser Double
                -- sMAP timestamps are usually milliseconds or seconds. 
                -- Legacy readingdb used seconds.
                return $ Reading (posixSecondsToUTCTime $ realToFrac ts) val

instance ToJSON Reading where
    toJSON Reading{..} = toJSON [toJSON (realToFrac (utcTimeToPOSIXSeconds readingTime) :: Double), toJSON readingValue]

-- | Data for a single stream in an ingestion payload
data StreamData = StreamData
    { sdUuid     :: UUID
    , sdReadings :: [Reading]
    , sdMetadata :: Map Text Text
    } deriving (Show, Eq, Generic)

instance FromJSON StreamData where
    parseJSON = withObject "StreamData" $ \obj -> do
        sdUuid <- obj .: "uuid"
        sdReadings <- obj .:? "Readings" .!= []
        
        -- Everything else is metadata
        let metadata = M.fromList $ flattenMetadata "" (Object obj)
        -- Remove uuid and Readings from metadata as they are special
        let sdMetadata = M.delete "uuid" $ M.delete "Readings" metadata
        
        return StreamData{..}

-- Helper to flatten nested JSON metadata into sMAP path format (e.g. Properties/UnitofTime)
flattenMetadata :: Text -> Value -> [(Text, Text)]
flattenMetadata prefix (Object obj) = concatMap (\(k, v) -> flattenMetadata (combine prefix (K.toText k)) v) (KM.toList obj)
  where
    combine "" k = k
    combine p k = p <> "/" <> k
flattenMetadata prefix (String s) = [(prefix, s)]
flattenMetadata prefix (Number n) = [(prefix, T.pack $ show n)]
flattenMetadata prefix (Bool b)   = [(prefix, T.pack $ show b)]
flattenMetadata _ Null           = []
flattenMetadata prefix (Array _)  = [(prefix, "<array>")] -- sMAP usually doesn't have arrays in metadata

instance ToJSON StreamData where
    toJSON StreamData{..} = object $
        [ "uuid" .= sdUuid
        , "Readings" .= sdReadings
        ] ++ map (\(k, v) -> K.fromText k .= v) (M.toList sdMetadata)

-- | The top-level payload is a map of Path to StreamData
type SmapPayload = Map Text StreamData
